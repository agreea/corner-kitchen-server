#!/bin/bash

NORMAL="\033[m"
BLACK="\033[0;30m"
RED="\033[01;31m"
GREEN="\033[01;32m"
YELLOW="\033[01;33m"
BLUE="\033[01;34m"
PURPLE="\033[01;35m"
AQUA="\033[01;36m"

if [ $# -ne 2 ]; then
    echo -e "${RED}Must call this script with the deploy path${NORMAL}"
    exit 1
fi

WORKDIR="$1"
ENV="$2"
BINARY="cornerd"

export GOPATH="$WORKDIR"

PIDFILE=$WORKDIR/$BINARY.pid
DAEMON=$WORKDIR/$BINARY

function colorize() {
    echo -e "${AQUA}>${NORMAL} ${YELLOW}$@${NORMAL}"
    "$@"
    return $?
}


# Set version while we are still in the git directory
echo -e "${AQUA}>${NORMAL} ${YELLOW}Setting version names${NORMAL}"
export VERSION="$(git rev-parse HEAD)"

colorize pushd "$WORKDIR"

echo -e "${AQUA}>${NORMAL} ${YELLOW}Starting deploy in `pwd`${NORMAL}"

SERVER_OPTS="-c /$WORKDIR/cornerd.gcfg"
if [ $ENV == "QA" ]; then
    SERVER_OPTS="-c /$WORKDIR/cornerd.qa.gcfg"
fi

colorize make
colorize /sbin/start-stop-daemon -p "$PIDFILE" -K -R INT/10/KILL --oknodo
colorize /sbin/start-stop-daemon -m -p "$PIDFILE" -b -S -a "$WORKDIR"/$BINARY -- $SERVER_OPTS
