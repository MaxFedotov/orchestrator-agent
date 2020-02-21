#!/bin/sh

chmod 744 /var/log/orchestrator-agent
chown -R orchestrator-agent:orchestrator-agent /var/log/orchestrator-agent
chown orchestrator-agent:orchestrator-agent /etc/orchestrator-agent.conf"

exit 0