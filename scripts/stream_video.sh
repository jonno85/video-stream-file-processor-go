#!/bin/zsh

# URL of the MP4 file
MP4_URLS=(
  "https://getsamplefiles.com/download/mp4/sample-1.mp4"
  "https://download.samplelib.com/mp4/sample-20s.mp4"
  "https://download.samplelib.com/mp4/sample-30s.mp4"
)

# Destination folder
DEST_FOLDER="./input-videos"

# Ensure the folder exists
mkdir -p "$DEST_FOLDER"

# Stream and save the file
for URL in "${MP4_URLS[@]}"; do
  FILE_NAME=$(basename "$URL")
  echo "Downloading $FILE_NAME from $URL..."
  
  echo "Downloading $URL..."
  curl -k -o "$DEST_FOLDER/$FILE_NAME" "$URL"
done
