#!/bin/bash
set -e

# Rebuild and rerun the headless backend on any Go file change.
cd $(dirname $0)
export JWT_SECRET_KEY=dev
export LOGLEVEL=${1:-DEBUG}
#export RM_ADMIN_API_TOKEN=devtoken
#export STORAGE_URL=http://$(hostname):3000
find . -iname "*.go" | entr -r make run
