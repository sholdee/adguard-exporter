FROM python:3-alpine

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache --virtual .build-deps gcc musl-dev

# Copy and install requirements
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Remove build dependencies
RUN apk del .build-deps

# Copy the script
COPY adguard_exporter.py .

# Expose the metrics port
EXPOSE 8000

# Set environment variables (can be overridden at runtime)
ENV LOG_FILE_PATH=/opt/adguardhome/work/data/querylog.json
ENV METRICS_PORT=8000

# Run the exporter
CMD ["python", "./adguard_exporter.py"]
