#!/bin/bash
for ((i=1; i <= 5 ; i++)); do
    echo "Stdout $i"
    echo "Stderr $i" >&2
    sleep 1
done
