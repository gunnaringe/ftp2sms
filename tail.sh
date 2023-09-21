#!/bin/bash

clear
folder_to_watch="data"

# Function to tail a file
tail_file() {
    local file="$1"
    tail -f "$file" &
}

# Initially tail all existing files
for file in "$folder_to_watch"/*; do
    if [ -f "$file" ]; then
        tail_file "$file"
    fi
done

# Use fswatch to monitor for new or modified files and tail them
fswatch -0 "$folder_to_watch" | while read -d "" event; do
    if [ -f "$event" ]; then
        tail_file "$event"
    fi
done

