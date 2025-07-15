#!/bin/bash
set -e  # Exit on error

kill_process_on_port() {
    PID=$(sudo lsof -t -i:8080 2>/dev/null || echo "")
    if [ -n "$PID" ]; then
        echo "Killing process (PID: $PID) on port 8080..."
        sudo kill -9 "$PID"
        sleep 1
    else
        echo "No process found on port 8080. Continuing..."
    fi
}

# Start deployment
echo "Starting deployment script..."
kill_process_on_port

# Update repository
echo "Pulling latest changes from git..."
git pull || { echo "Git pull failed!"; exit 1; }

# Build the Go application
echo "Building Go application..."
go build main.go || { echo "Go build failed!"; exit 1; }

# Start the Go application
echo "Starting Go application..."
sudo -g carbn-access nohup ./main >> go_output.txt 2>&1 &

# Verify Go application is running
sleep 2
NEW_PID=$(sudo lsof -t -i:8080 2>/dev/null)

if [ -n "$NEW_PID" ]; then
    echo "Go application started successfully (PID: $NEW_PID)"
else
    echo "Warning: Go application may not have started correctly"
    echo "Check go_output.txt for details"
fi

echo "Deployment complete!"
