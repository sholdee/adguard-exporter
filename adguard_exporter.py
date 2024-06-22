import time
import json
import os
from prometheus_client import start_http_server, Counter

# Define a single Prometheus metric
dns_queries = Counter('dns_queries', 'Details of DNS queries', ['qh', 'ip', 'qt', 'response_size', 'result_reason', 'status', 'upstream'])

log_file_path = 'querylog.json'
position_file_path = '.position'

def get_last_position():
    try:
        with open(position_file_path, 'r') as f:
            pos = int(f.read().strip())
            inode = os.stat(log_file_path).st_ino
            print(f"Read last position: {pos}, inode: {inode}")
            return pos, inode
    except (FileNotFoundError, ValueError) as e:
        print(f"Error reading last position: {e}")
        return 0, None

def save_last_position(pos, inode):
    with open(position_file_path, 'w') as f:
        f.write(f"{pos}\n{inode}")
    print(f"Saved position: {pos}, inode: {inode}")

def read_new_lines(file, start_pos):
    file.seek(start_pos)
    lines = file.readlines()
    new_pos = file.tell()
    print(f"Read {len(lines)} new lines, new position: {new_pos}")
    return lines, new_pos

def reset_metrics():
    # Reset the Counter metrics
    dns_queries._metrics.clear()
    print("Metrics reset")

def parse_and_export(lines):
    for line in lines:
        if line.strip():
            try:
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
                print(f"Exported metric for query: {data.get('QH', 'unknown')}")
            except json.JSONDecodeError as e:
                print(f"Error decoding JSON: {e}, line: {line}")

if __name__ == '__main__':
    # Start the Prometheus metrics server
    start_http_server(8000)
    print("Prometheus metrics server started on port 8000")

    # Wait for the log file to exist
    while not os.path.exists(log_file_path):
        print(f"Waiting for {log_file_path} to be created...")
        time.sleep(10)

    print(f"Log file {log_file_path} found")

    # Get the last read position and inode
    last_position, last_inode = get_last_position()

    while True:
        try:
            current_inode = os.stat(log_file_path).st_ino

            # Check for log rotation
            if last_inode and last_inode != current_inode:
                last_position = 0
                reset_metrics()
                print(f"Log file rotated, resetting position to {last_position}")

            with open(log_file_path, 'r') as log_file:
                new_lines, new_position = read_new_lines(log_file, last_position)
                if new_lines:
                    parse_and_export(new_lines)
                    save_last_position(new_position, current_inode)

            # Update last position and inode
            last_position = new_position
            last_inode = current_inode

        except Exception as e:
            print(f"Error during processing: {e}")

        # Sleep for a while before reading the log again
        time.sleep(10)
