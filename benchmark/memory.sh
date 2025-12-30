#!/bin/bash

TARGET_BIN="chitosocket"
INTERVAL=2  # Seconds between checks

echo "=========================================================================="
echo " Monitoring: $TARGET_BIN"
echo " Press [CTRL+C] to exit"
echo "=========================================================================="
# Table Header
printf "%-19s | %-5s | %-6s | %-8s | %-8s | %-4s | %-8s\n" "TIMESTAMP" "PID" "CPU %" "RSS (MB)" "VSZ (MB)" "STAT" "STARTED"
echo "--------------------|-------|--------|----------|----------|------|---------"

while true; do
    # Find the PID of the binary
    PID=$(pgrep -x "$TARGET_BIN")

    if [ -z "$PID" ]; then
        TIMESTAMP=$(date "+%Y-%m-%d %H:%M:%S")
        printf "%-19s | %-5s | %-6s | %-8s | %-8s | %-4s | %-8s\n" "$TIMESTAMP" "N/A" "0.0" "OFFLINE" "OFFLINE" "-" "N/A"
    else
        # Fetch stats using ps
        # rss (Resident Set Size), %cpu, %mem, vsz (Virtual Size), stat (State), start (Start Time)
        STATS=$(ps -p "$PID" -o rss,%cpu,vsz,stat,start --no-headers)
        
        # Read stats into variables
        read -r RSS PCPU VSZ STAT START <<< "$STATS"
        
        # Convert KB to MB (rounded to 2 decimal places)
        RSS_MB=$(echo "scale=2; $RSS/1024" | bc 2>/dev/null || awk "BEGIN {print $RSS/1024}")
        VSZ_MB=$(echo "scale=2; $VSZ/1024" | bc 2>/dev/null || awk "BEGIN {print $VSZ/1024}")
        
        TIMESTAMP=$(date "+%H:%M:%S")
        
        # Output row
        printf "%-19s | %-5s | %-6s | %-8s | %-8s | %-4s | %-8s\n" \
               "$(date "+%Y-%m-%d %H:%M:%S")" "$PID" "$PCPU%" "$RSS_MB" "$VSZ_MB" "$STAT" "$START"
    fi

    sleep "$INTERVAL"
done
