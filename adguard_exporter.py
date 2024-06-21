from prometheus_client import start_http_server, Gauge
import json
import time
import os

# Define Prometheus metric
dns_queries = Gauge('dns_queries', 'DNS query metrics', ['qh', 'qt', 'upstream', 'result_reason', 'ip', 'status', 'response_size'])

def process_log_line(line):
    data = json.loads(line)
    qh = data.get('QH', 'unknown')
    qt = data.get('QT', 'unknown')
    upstream = data.get('Upstream', 'unknown')
    result = data.get('Result', {})
    result_reason = result.get('Reason', 'unknown')
    elapsed = data.get('Elapsed', 0)
    ip = data.get('IP', 'unknown')
    answer = data.get('Answer', '')
    response_size = len(answer)
    status = 'success' if not result else 'failure'

    # Check if the query was blocked
    if result.get('IsFiltered', False):
        status = 'blocked'

    # Update Prometheus metric
    dns_queries.labels(qh, qt, upstream, result_reason, ip, status, response_size).set(elapsed)

def read_querylog():
    file_path = '/opt/adguardhome/work/data/querylog.json'
    with open(file_path, 'r') as file:
        for line in file:
            process_log_line(line)

def main():
    # Start up the server to expose the metrics.
    start_http_server(8000)
    while True:
        read_querylog()
        time.sleep(60)

if __name__ == '__main__':
    main()
