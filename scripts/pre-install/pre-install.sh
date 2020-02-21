#!/bin/sh
getent group orchestrator-agent >/dev/null || groupadd -r orchestrator-agent
getent passwd orchestrator-agent >/dev/null || \
    useradd -r -g orchestrator-agent -s /sbin/nologin \
    -c "Orchestrator-agent service" orchestrator-agent
exit 0