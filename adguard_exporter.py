import time
import os
import sys
import threading
import logging
import signal
import configparser
from prometheus_client import make_wsgi_app, Counter, Gauge
from collections import Counter as CollectionsCounter, defaultdict
from wsgiref.simple_server import make_server, WSGIRequestHandler
import heapq
import orjson
from watchdog.observers import Observer
from watchdog.events import FileSystemEventHandler

# Configuration
config = configparser.ConfigParser()
config.read_dict({
    'DEFAULT': {
        'LOG_FILE_PATH': '/opt/adguardhome/work/data/querylog.json',
        'METRICS_PORT': '8000',
        'LOG_LEVEL': 'INFO'
    }
})

log_file_path = os.environ.get('LOG_FILE_PATH', config['DEFAULT']['LOG_FILE_PATH'])
metrics_port = int(os.environ.get('METRICS_PORT', config['DEFAULT']['METRICS_PORT']))
log_level_str = os.environ.get('LOG_LEVEL', config['DEFAULT']['LOG_LEVEL'])

# Convert string log level to logging constant
log_level = getattr(logging, log_level_str.upper(), logging.INFO)

# Set up logging
log_format = '%(asctime)s - %(levelname)s - %(message)s'
logging.basicConfig(level=log_level, format=log_format, 
                    handlers=[logging.StreamHandler(sys.stdout)])
logger = logging.getLogger(__name__)

# Set the level for the wsgiref.simple_server logger
logging.getLogger('wsgiref.simple_server').setLevel(log_level)

logger.info(f"Using log file path: {log_file_path}")
logger.info(f"Using metrics port: {metrics_port}")
logger.info(f"Log level set to: {logging.getLevelName(log_level)}")

# Define Prometheus metrics
dns_queries = Counter('agh_dns_queries', 'Total number of DNS queries')
blocked_queries = Counter('agh_blocked_dns_queries', 'Total number of blocked DNS queries')
query_types = Counter('agh_dns_query_types', 'Types of DNS queries', ['query_type'])
top_hosts = Counter('agh_dns_query_hosts', 'Top DNS query hosts', ['host'])
top_blocked_hosts = Counter('agh_blocked_dns_query_hosts', 'Top blocked DNS query hosts', ['host'])
safe_search_enforced_hosts = Counter('agh_safe_search_enforced_hosts', 'Safe search enforced hosts', ['host'])
average_response_time = Gauge('agh_dns_average_response_time', 'Average response time for DNS queries in milliseconds')
average_upstream_response_time = Gauge('agh_dns_average_upstream_response_time', 'Average response time by upstream server', ['server'])

class CustomRequestHandler(WSGIRequestHandler):
    def log_request(self, code='-', size='-'):
        if str(code) == '200':
            logging.debug('"%s" %s %s', self.requestline, str(code), str(size))
        else:
            logging.warning('"%s" %s %s', self.requestline, str(code), str(size))

class TopHosts:
    def __init__(self, max_size=100):
        self.max_size = max_size
        self.counter = CollectionsCounter()
        self.heap = []
        self.lock = threading.Lock()

    def add(self, host):
        with self.lock:
            self.counter[host] += 1
            count = self.counter[host]
            self.heap = [(c, h) for c, h in self.heap if h != host]
            if len(self.heap) < self.max_size:
                heapq.heappush(self.heap, (count, host))
            elif count > self.heap[0][0]:
                heapq.heappushpop(self.heap, (count, host))

    def get_top(self):
        with self.lock:
            return sorted(self.heap, reverse=True)

class MetricsCollector:
    def __init__(self):
        self.top_hosts = TopHosts(max_size=100)
        self.top_blocked_hosts = TopHosts(max_size=100)
        self.response_times = []
        self.upstream_response_times = defaultdict(list)
        self.window_size = 300  # 5 minute window for response time averages
        self.lock = threading.Lock()

    def update_metrics(self, data):
        current_time = time.time()
        host = data.get('QH', 'unknown')
        query_type = data.get('QT', 'unknown')
        is_blocked = data.get('Result', {}).get('IsFiltered', False)
        result_reason = str(data.get('Result', {}).get('Reason', 'unknown'))
        elapsed_ns = data.get('Elapsed', 0)
        upstream = data.get('Upstream', 'unknown')

        dns_queries.inc()
        query_types.labels(query_type).inc()

        if not is_blocked:
            self.top_hosts.add(host)
        
        elapsed_ms = elapsed_ns / 1_000_000  # Convert nanoseconds to milliseconds
        with self.lock:
            self.response_times.append((current_time, elapsed_ms))

        if upstream != 'unknown':
            with self.lock:
                self.upstream_response_times[upstream].append((current_time, elapsed_ms))

        if is_blocked and result_reason == '7':
            safe_search_enforced_hosts.labels(host).inc()
        elif is_blocked:
            blocked_queries.inc()
            self.top_blocked_hosts.add(host)

        self.process_metrics()

    def process_metrics(self):
        current_time = time.time()
        cutoff_time = current_time - self.window_size

        # Update top hosts metrics
        top_hosts._metrics.clear()
        for count, host in self.top_hosts.get_top():
            top_hosts.labels(host).inc(count)

        # Update top blocked hosts metrics
        top_blocked_hosts._metrics.clear()
        for count, host in self.top_blocked_hosts.get_top():
            top_blocked_hosts.labels(host).inc(count)

        with self.lock:
            recent_response_times = [rt for t, rt in self.response_times if t > cutoff_time]
            if recent_response_times:
                avg_response_time = sum(recent_response_times) / len(recent_response_times)
                average_response_time.set(avg_response_time)

            for upstream, times in self.upstream_response_times.items():
                recent_times = [rt for t, rt in times if t > cutoff_time]
                if recent_times:
                    avg_upstream_time = sum(recent_times) / len(recent_times)
                    average_upstream_response_time.labels(upstream).set(avg_upstream_time)

            self.response_times = [(t, rt) for t, rt in self.response_times if t > cutoff_time]
            for upstream in self.upstream_response_times:
                self.upstream_response_times[upstream] = [(t, rt) for t, rt in self.upstream_response_times[upstream] if t > cutoff_time]

class LogHandler(FileSystemEventHandler):
    def __init__(self, log_file_path, metrics_collector):
        self.log_file_path = log_file_path
        self.metrics_collector = metrics_collector
        self.last_position = 0
        self.health_status = True
        self.file_exists = os.path.exists(log_file_path)
        self.initial_load()

    def initial_load(self):
        if self.file_exists:
            logger.info(f"Loading existing log file: {self.log_file_path}")
            self.process_new_lines()
        else:
            logger.info(f"Waiting for log file: {self.log_file_path}")

    def on_created(self, event):
        if event.src_path == self.log_file_path:
            logger.info(f"Log file created: {self.log_file_path}")
            self.file_exists = True
            self.last_position = 0
            self.process_new_lines()

    def on_deleted(self, event):
        if event.src_path == self.log_file_path:
            logger.info(f"Log file deleted: {self.log_file_path}")
            self.file_exists = False
            self.last_position = 0

    def on_modified(self, event):
        if event.src_path == self.log_file_path:
            self.process_new_lines()

    def process_new_lines(self):
        try:
            with open(self.log_file_path, 'r') as log_file:
                log_file.seek(self.last_position)
                lines = log_file.readlines()
                logger.debug(f"Processing {len(lines)} new lines")
                for line in lines:
                    if line.strip():
                        try:
                            data = orjson.loads(line)
                            self.metrics_collector.update_metrics(data)
                        except orjson.JSONDecodeError:
                            logger.error(f"Error decoding JSON: {line}")
                self.last_position = log_file.tell()
            self.health_status = True
        except FileNotFoundError:
            logger.error(f"Log file not found: {self.log_file_path}")
            self.file_exists = False
            self.health_status = False
        except Exception as e:
            logger.error(f"Error processing log file: {e}")
            self.health_status = False

    def is_healthy(self):
        return self.health_status

class HealthServer:
    def __init__(self, log_handler):
        self.log_handler = log_handler
        self.server_ready = False

    def set_ready(self):
        self.server_ready = True

    def livez(self, environ, start_response):
        status = '200 OK' if self.log_handler.is_healthy() else '503 Service Unavailable'
        headers = [('Content-type', 'text/plain; charset=utf-8')]
        start_response(status, headers)
        return [b"Alive" if status == '200 OK' else b"Unhealthy"]

    def readyz(self, environ, start_response):
        status = '200 OK' if self.server_ready else '503 Service Unavailable'
        headers = [('Content-type', 'text/plain; charset=utf-8')]
        start_response(status, headers)
        return [b"Ready" if status == '200 OK' else b"Not Ready"]

def start_combined_server(port, health_server):
    def combined_app(environ, start_response):
        if environ['PATH_INFO'] == '/livez':
            return health_server.livez(environ, start_response)
        elif environ['PATH_INFO'] == '/readyz':
            return health_server.readyz(environ, start_response)
        return make_wsgi_app()(environ, start_response)

    httpd = make_server('', port, combined_app, handler_class=CustomRequestHandler)
    logger.info(f"Combined server started on port {port}, /metrics, /livez, and /readyz endpoints")
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    return httpd

def graceful_shutdown(signum, frame):
    logger.info("Received shutdown signal. Exiting...")
    observer.stop()
    observer.join()
    combined_server.shutdown()
    sys.exit(0)

if __name__ == '__main__':
    metrics_collector = MetricsCollector()
    log_handler = LogHandler(log_file_path, metrics_collector)
    health_server = HealthServer(log_handler)

    try:
        combined_server = start_combined_server(metrics_port, health_server)

        observer = Observer()
        observer.schedule(log_handler, path=os.path.dirname(log_file_path), recursive=False)
        observer.start()

        health_server.set_ready()
    except Exception as e:
        logger.error(f"Failed to start exporter: {e}")
        sys.exit(1)

    signal.signal(signal.SIGTERM, graceful_shutdown)
    signal.signal(signal.SIGINT, graceful_shutdown)

    logger.info(f"Exporter is ready. Monitoring log file: {log_file_path}")

    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        pass
    finally:
        graceful_shutdown(None, None)
