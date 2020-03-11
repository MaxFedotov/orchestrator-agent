import pytest
import os
import vagrant
import subprocess
import sys
from os import path
import shutil

def pytest_addoption(parser):
    parser.addoption("--mysql_version", action="store", choices=["57", "80"], default="57")
    parser.addoption("--orch_repo", action="store", default="https://github.com/openark/orchestrator.git")
    parser.addoption("--orch_branch", action="store", default="master")
    parser.addoption("--only-update-agents", action="store_true", default=False)
    parser.addoption("--no-destroy-vagrant-boxes", action="store_true", default=False)

def pytest_configure():
    pytest.vagrant_hosts = None

@pytest.fixture(scope="module")
def enable_gtid():
    for host, box in pytest.vagrant_hosts.items():
        if 'source' in host.lower():
            print("Enabling gtid {}".format(host))
            box.ssh(command="sudo crudini --set /etc/my.cnf mysqld gtid_mode ON && sudo crudini --set /etc/my.cnf mysqld enforce-gtid-consistency ON && sudo service mysql restart")
            box.ssh(command="mysql -BNe \"RESET MASTER\"")
            box.ssh(command="mysql employees -BNe \"UPDATE employees set first_name='test' WHERE emp_no = 10001\"")
        if 'target' in host.lower():
            print("Enabling gtid {}".format(host))
            box.ssh(command="sudo crudini --set /etc/my.cnf mysqld gtid_mode ON && sudo crudini --set /etc/my.cnf mysqld enforce-gtid-consistency ON && sudo service mysql restart")

@pytest.fixture(scope="module")
def disable_gtid():
    pass
    for host, box in pytest.vagrant_hosts.items():
        if 'source' in host.lower():
            print("Disabling gtid {}".format(host))
            box.ssh(command="sudo crudini --del /etc/my.cnf mysqld gtid_mode && sudo crudini --del /etc/my.cnf mysqld enforce-gtid-consistency && sudo service mysql restart")
            box.ssh(command="mysql -BNe \"RESET MASTER;\"")
            box.ssh(command="mysql employees -BNe \"UPDATE employees set first_name='test' WHERE emp_no = 10001\"")
        if 'target' in host.lower():
            print("Disabling gtid {}".format(host))
            box.ssh(command="sudo crudini --del /etc/my.cnf mysqld gtid_mode && sudo crudini --del /etc/my.cnf mysqld enforce-gtid-consistency && sudo service mysql restart")

@pytest.fixture(scope="module")
def reset_lvm():
    for host, box in pytest.vagrant_hosts.items():
        if 'source' in host.lower():
            print("Reseting LVM {}".format(host))
            box.ssh(command="sudo service mysql stop")
            box.ssh(command="sudo mkdir /var/lib/mysql2")
            box.ssh(command="sudo bash -c \"cp -R /var/lib/mysql/* /var/lib/mysql2\"")
            box.ssh(command="sudo umount /var/lib/mysql || true")
            box.ssh(command="sudo rm -rf /var/lib/mysql || true")
            box.ssh(command="sudo lvremove -f mysql_vg")
            box.ssh(command="sudo lvcreate -L 900M -n mysql_lv mysql_vg -y")
            box.ssh(command="sudo mkfs.ext4 /dev/mysql_vg/mysql_lv")
            box.ssh(command="sudo mkdir /var/lib/mysql || true")
            box.ssh(command="sudo mount /dev/mysql_vg/mysql_lv /var/lib/mysql")
            box.ssh(command="sudo bash -c \"cp -R /var/lib/mysql2/* /var/lib/mysql\"")
            box.ssh(command="sudo rm -rf /var/lib/mysql2")
            box.ssh(command="sudo chown -R mysql:mysql /var/lib/mysql")
            box.ssh(command="sudo service mysql start && sleep 10s")

@pytest.fixture(autouse=True)
def reset_target_agent():
    for host, box in pytest.vagrant_hosts.items():
        if 'target' in host.lower():
            print("Resetting agent {}".format(host))
            box.ssh(command="mysql -e 'STOP SLAVE;'")
            box.ssh(command="mysql -e 'RESET SLAVE ALL;'")
            box.ssh(command="mysql -e 'DROP DATABASE IF EXISTS employees;'")
            box.ssh(command="mysql -e 'DROP DATABASE IF EXISTS sakila;'")
            box.ssh(command="mysql -e 'DROP DATABASE IF EXISTS world;'")
            box.ssh(command="mysql -e 'RESET MASTER;'")


@pytest.fixture(scope="module")
def prepare_env(pytestconfig):
    vagrant_hosts = {
        "orchestrator": None,
        "sourceagent": None,
        "targetagent": None
    }
    hosts_records = []
    vagrantPath = os.path.join(os.getcwd(), "vagrant/vagrantfiles/mysql_{}/".format(pytestconfig.getoption("mysql_version")))
    print("Will use vagrant file: {}".format(vagrantPath))
    print("Will use {} repo {} branch for Orchestrator".format(pytestconfig.getoption("orch_repo"), pytestconfig.getoption("orch_branch")))
    for index, host in enumerate(vagrant_hosts):
        ip_address = "192.168.58.2{}".format(index)
        vagrant_box = create_vagrant_box(vagrantPath, host, ip_address)
        vagrant_hosts[host] = vagrant_box
        hosts_records.append("{} {}".format(ip_address, host))

    for host, box in vagrant_hosts.items():
        hosts_record = "\n".join(hosts_records)
        box.ssh(command="sudo bash -c \"echo '{}' >> /etc/hosts\"".format(hosts_record))
    
    if pytestconfig.getoption("only_update_agents") == False:
        prepare_orchestrator(vagrant_hosts["orchestrator"], pytestconfig.getoption("orch_repo"), pytestconfig.getoption("orch_branch"))
    
    print("Building orchestrator-agent packages")
    orchestrator_dir = path.abspath(path.join(os.getcwd(),"../../"))
    process = subprocess.Popen(["goreleaser", "--snapshot" ,"--rm-dist"], cwd=orchestrator_dir, stdout=sys.stdout)
    process.wait()
    os.popen("rm -rf $(find vagrant/shared -name 'orchestrator-agent*.rpm')")
    os.popen("cp $(find {} -name 'orchestrator-agent*.rpm' | grep -v sysv) vagrant/shared".format(orchestrator_dir))

    server_id = 2
    for host, box in vagrant_hosts.items():
        if 'agent' in host.lower():
            print("Preparing {}".format(host))
            prepare_agent(box, pytestconfig.getoption("only_update_agents"), server_id, pytestconfig.getoption("mysql_version"))
            server_id += 1
    
    for host, box in vagrant_hosts.items():
        pass
        if 'source' in host.lower():
            print("Creating databases on {}".format(host))
            print(box.ssh(command="cd /home/vagrant/databases/employees && mysql < employees.sql"))
            print(box.ssh(command="mysql < /home/vagrant/databases/sakila/sakila.sql"))
            print(box.ssh(command="mysql < /home/vagrant/databases/world/world.sql"))

    pytest.vagrant_hosts = vagrant_hosts
    yield vagrant_hosts
    if pytestconfig.getoption("no_destroy_vagrant_boxes") == False:
        for host, box in vagrant_hosts.items():
            print("Destroing {} vagrant box".format(host))
            box.destroy()

def create_vagrant_box(vagrantPath, hostname, ip):
    print("Creating vagrant box for {}".format(hostname))
    os_env = os.environ.copy()
    os_env['hostname'] = hostname
    os_env['ip'] = ip
    v = vagrant.Vagrant(root=vagrantPath, env=os_env)
    bootstrap = v.up(stream_output=True)
    for line in bootstrap:
        print(line.rstrip())
    return v

def prepare_orchestrator(orchestrator, repo, branch):
    print("Preparing orchestrator")
    print(orchestrator.ssh(command="mkdir -p $GOPATH/src/github.com/github/orchestrator && git clone --single-branch --branch {} {} $GOPATH/src/github.com/github/orchestrator".format(branch, repo)))
    print(orchestrator.ssh(command="cd $GOPATH/src/github.com/github/orchestrator/ && ./build.sh -t linux -i systemd -P"))
    print(orchestrator.ssh(command="sudo yum install -y $(find /tmp/orchestrator-release/ -name 'orchestrator-*.rpm' | grep -v cli)"))
    orchestrator.ssh(command="sudo cp /vagrant/orchestrator.conf.json /etc/orchestrator.conf.json")
    orchestrator.ssh(command="sudo service orchestrator start")


def prepare_agent(agent, update_agent,server_id, mysql_version):
    if update_agent:
        print(agent.ssh(command="sudo yum -y erase orchestrator-agent.x86_64"))
        agent.ssh(command="sudo service orchestrator-agent stop")
    else:
        agent.ssh(command="sudo bash -c \"grep -rli /etc/my.cnf -e 'server_id = 1' |  xargs -i@ sed -i 's/server_id = 1/server_id = {}/g' @\"".format(server_id))
        agent.ssh(command="sudo rm -rf /var/lib/mysql/auto.cnf")
        agent.ssh(command="sudo service mysql restart")
    agent.ssh(command="sudo bash -c \"rm -rf /tmp/bkp && mkdir /tmp/bkp && chown -R mysql:mysql /tmp/bkp\"")
    print(agent.ssh(command="sudo yum install -y $(find /vagrant -name 'orchestrator-agent*.rpm')"))
    agent.ssh(command="sudo cp /vagrant/orchestrator-agent_{}.conf /etc/orchestrator-agent.conf && sudo chown mysql:mysql /etc/orchestrator-agent.conf".format(mysql_version))
    agent.ssh(command="sudo systemctl daemon-reload")
    agent.ssh(command="sudo service orchestrator-agent start && sleep 10s")

