# Use the official Python image from the Docker Hub
FROM python:3.10-slim

# Set the working directory in the container
WORKDIR /app

# Copy the requirements file into the container
COPY requirements.txt .

# Install any needed packages specified in requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

# Copy the rest of the application into the container
COPY adguard_exporter.py .

# Make port 8000 available to the world outside this container
EXPOSE 8000

# Define the command to run the application
CMD ["python", "./adguard_exporter.py"]
