import time
import json
import os
from prometheus_client import start_http_server, Counter

# Define a single Prometheus metric
dns_queries = Counter('dns_queries', 'Details of DNS queries', ['qh', 'ip', 'qt', 'response_size', 'result_reason', 'status', 'upstream'])

log_file_path = '/opt/adguardhome/work/data/querylog.json'
position_file_path = '/opt/adguardhome/work/data/.position'

def ensure_query_log_exists():
    if not os.path.exists(log_file_path):
        with open(log_file_path, 'w') as f:
            json.dump([], f)

def get_last_position():
    try:
        with open(position_file_path, 'r') as f:
            pos = int(f.read().strip())
            inode = os.stat(log_file_path).st_ino
            return pos, inode
    except (FileNotFoundError, ValueError):
        return 0, None

def save_last_position(pos, inode):
    with open(position_file_path, 'w') as f:
        f.write(f"{pos}\n{inode}")

def read_new_lines(file, start_pos):
    file.seek(start_pos)
    lines = file.readlines()
    new_pos = file.tell()
    return lines, new_pos

def reset_metrics():
    # Reset the Counter metrics
    dns_queries._metrics.clear()

def parse_and_export(lines):
    for line in lines:
        if line.strip():
            data = json.loads(line)
            dns_queries.labels(
                qh=data.get('QH', 'unknown'),
                ip=data.get('IP', 'unknown'),
                qt=data.get('QT', 'unknown'),
                response_size=str(len(data.get('Answer', ''))),
                result_reason=str(data.get('Result', {}).get('Reason', 'unknown')),
                status='blocked' if data.get('Result', {}).get('IsFiltered', False) else 'success',
                upstream=data.get('Upstream', 'unknown')
            ).inc()

if __name__ == '__main__':
    # Ensure the query log file exists
    ensure_query_log_exists()

    # Start the Prometheus metrics server
    start_http_server(8000)

    # Get the last read position and inode
    last_position, last_inode = get_last_position()

    while True:
        current_inode = os.stat(log_file_path).st_ino

        # Check for log rotation
        if last_inode and last_inode != current_inode:
            last_position = 0
            reset_metrics()

        with open(log_file_path, 'r') as log_file:
            new_lines, new_position = read_new_lines(log_file, last_position)
            if new_lines:
                parse_and_export(new_lines)
                save_last_position(new_position, current_inode)

        # Update last position and inode
        last_position = new_position
        last_inode = current_inode

        # Sleep for a while before reading the log again
        time.sleep(10)
