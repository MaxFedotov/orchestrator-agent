#!/bin/bash

set -e

usage() {
  echo
  echo "Usage: $0 [-h] [-v]"
  echo "Options:"
  echo "-h Show this screen"
  echo "-v rpm package version"
  echo
}

build() {
    local package=$1
    local version=$2
    rm -rf dist/rpm_build
    mkdir -p dist/rpm_build
    mkdir -p dist/rpm_build/var/log/orchestrator-agent
    mkdir -p dist/rpm_build/usr/bin
    if [ $package = "systemd" ]; then
        mkdir -p dist/rpm_build/etc/systemd/system
        cp etc/systemd/orchestrator-agent.service dist/rpm_build/etc/systemd/system
        local package_name="orchestrator-agent"
    elif [ $package = "sysv" ]; then
        mkdir -p dist/rpm_build/etc/init.d
        cp etc/init.d/orchestrator-agent.bash dist/rpm_build/etc/init.d/
        local package_name="orchestrator-agent-sysv"
    else 
        echo "Wrong init system"
        exit 1
    fi
    cp dist/orchestrator-agent_linux_amd64/orchestrator-agent dist/rpm_build/usr/bin
    cp conf/orchestrator-agent.conf dist/rpm_build/etc

    fpm -v $version --epoch 1 -f -s dir -n $package_name -m "MaxFedotov <m.a.fedotov@gmail.com>" --description "orchestrator-agent: MySQL management agent" --url "https://github.com/github/orchestrator-agent" --vendor "Github" --license "Apache 2.0" --config-files /etc/orchestrator-agent.conf --rpm-os linux --before-install scripts/pre-install/pre-install.sh --after-install scripts/post-install/post-install.sh --rpm-attr 744,mysql,mysql:/var/log/orchestrator-agent -d socat -t rpm -C dist/rpm_build -p dist/

    rm -rf dist/rpm_build
}

while getopts "s:p:v:h" opt; do
  case $opt in
  s)
    source="${OPTARG}"
    ;;
  h)
    usage
    exit 0
    ;;
  p)
    package="${OPTARG}"
    ;;
  v)
    version="${OPTARG}"
    ;;
  ?)
    usage
    exit 2
    ;;
  esac
done

shift $(( OPTIND - 1 ));

if [ -z "$version" ]; then
    echo "Error. Version not specified"
    exit 1
fi

build "systemd" "$version"
build "sysv" "$version"


