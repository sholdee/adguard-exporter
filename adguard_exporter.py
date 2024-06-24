import time
import json
import os
import sys
from prometheus_client import start_http_server, Counter
from collections import Counter as CollectionsCounter

# Define separate Prometheus metrics
dns_queries = Counter('dns_queries', 'Total number of DNS queries')
blocked_queries = Counter('blocked_dns_queries', 'Total number of blocked DNS queries')
query_types = Counter('dns_query_types', 'Types of DNS queries', ['query_type'])
top_hosts = Counter('dns_query_hosts', 'Top DNS query hosts', ['host'])
top_blocked_hosts = Counter('blocked_dns_query_hosts', 'Top blocked DNS query hosts', ['host'])
safe_search_enforced_hosts = Counter('safe_search_enforced_hosts', 'Safe search enforced hosts', ['host'])

# Define counters to track hosts
host_counter = CollectionsCounter()
blocked_host_counter = CollectionsCounter()

log_file_path = '/opt/adguardhome/work/data/querylog.json'
position_file_path = '/opt/adguardhome/work/data/.position'

def get_last_position():
    if os.path.exists(position_file_path):
        try:
            with open(position_file_path, 'r') as f:
                pos, inode = f.read().strip().split('\n')
                pos = int(pos)
                inode = int(inode)
                print(f"Read last position: {pos}, inode: {inode}")
                sys.stdout.flush()
                return pos, inode
        except (ValueError, OSError) as e:
            print(f"Error reading last position: {e}")
            sys.stdout.flush()
            return 0, None
    else:
        print("Position file not found, starting from the beginning.")
        sys.stdout.flush()
        return 0, None

def save_last_position(pos, inode):
    with open(position_file_path, 'w') as f:
        f.write(f"{pos}\n{inode}")

def read_new_lines(file, start_pos):
    file.seek(start_pos)
    lines = file.readlines()
    new_pos = file.tell()
    return lines, new_pos

def update_top_hosts(counter, metric, top_n):
    top_items = counter.most_common(top_n)
    metric._metrics.clear()
    for item in top_items:
        metric.labels(item[0]).inc(item[1])

def parse_and_export(lines):
    global host_counter, blocked_host_counter

    for line in lines:
        if line.strip():
            try:
                data = json.loads(line)
                host = data.get('QH', 'unknown')
                query_type = data.get('QT', 'unknown')
                is_blocked = data.get('Result', {}).get('IsFiltered', False)
                result_reason = str(data.get('Result', {}).get('Reason', 'unknown'))
                status = 'blocked' if is_blocked else 'success'

                dns_queries.inc()
                query_types.labels(query_type).inc()
                host_counter[host] += 1
                if is_blocked and result_reason == '3':
                    blocked_queries.inc()
                    blocked_host_counter[host] += 1
                if is_blocked and result_reason == '7':
                    safe_search_enforced_hosts.labels(host).inc()

                # Update Prometheus metrics with top 100 hosts
                update_top_hosts(host_counter, top_hosts, 100)
                update_top_hosts(blocked_host_counter, top_blocked_hosts, 100)
            except json.JSONDecodeError as e:
                print(f"Error decoding JSON: {e}, line: {line}")
                sys.stdout.flush()
                pass

if __name__ == '__main__':
    start_http_server(8000)
    print("Prometheus metrics server started on port 8000")
    sys.stdout.flush()
    while not os.path.exists(log_file_path):
        print(f"Waiting for {log_file_path} to be created...")
        sys.stdout.flush()
        time.sleep(10)

    print(f"Log file {log_file_path} found")
    sys.stdout.flush()
    last_position, last_inode = get_last_position()

    while True:
        try:
            current_inode = os.stat(log_file_path).st_ino

            if last_inode and last_inode != current_inode:
                last_position = 0
                print(f"Log file rotated, resetting position to {last_position}")
                sys.stdout.flush()

            with open(log_file_path, 'r') as log_file:
                new_lines, new_position = read_new_lines(log_file, last_position)
                if new_lines:
                    parse_and_export(new_lines)
                    save_last_position(new_position, current_inode)

            last_position = new_position
            last_inode = current_inode

        except Exception as e:
            print(f"Error during processing: {e}")
            sys.stdout.flush()

        time.sleep(10)
