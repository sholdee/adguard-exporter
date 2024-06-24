FROM python:3-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY adguard_exporter.py .
EXPOSE 8000
CMD ["python", "./adguard_exporter.py"]
