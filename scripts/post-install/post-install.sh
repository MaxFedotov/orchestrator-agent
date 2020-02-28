#!/bin/sh

chmod 744 /var/log/orchestrator-agent
chown -R mysql:mysql /var/log/orchestrator-agent
chown mysql:mysql /etc/orchestrator-agent.conf

exit 0