#!/bin/bash

set -e

APP_NAME="view-my-batteries"
LOG_FILE="app.log"

# Extract port from ADDR if set, default is 8080
PORT=${ADDR:--:8080}
PORT=${PORT##*:}

echo "Building $APP_NAME..."
go build -o "$APP_NAME" .

echo "Stopping any existing instance of $APP_NAME..."
# Kill by process name
pkill -f "./$APP_NAME" || true

# Kill any process using the target port
if [ -n "$PORT" ]; then
    PID=$(lsof -t -i :"$PORT" 2>/dev/null || true)
    if [ -n "$PID" ]; then
        echo "Killing process $PID on port $PORT..."
        kill -9 $PID || true
    fi
fi

echo "Starting $APP_NAME..."
nohup ./"$APP_NAME" > "$LOG_FILE" 2>&1 &
NEW_PID=$!

echo "Deployed successfully! Application running with PID $NEW_PID."
echo "Logs are being written to $LOG_FILE."
