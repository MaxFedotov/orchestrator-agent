#!/bin/bash
mysqlReplication=""
orchAgentConfig=""
orchAgentConfigFile=""
customLogDir=0


while getopts "r:c:l" opt; do
    case $opt in
    r) mysqlReplication="$OPTARG"
     ;;
    c) orchAgentConfig="$OPTARG"
     ;;
    l) customLogDir=1
     ;;
    esac
done

shift $(( OPTIND - 1 ));

echo "Running with following params. Replication: $mysqlReplication , customLogDir: $customLogDir"

echo "Disabling SELINUX"
sed -i 's/SELINUX=enforcing/SELINUX=disabled/g' /etc/selinux/config

echo "Creating directory for backups"
mkdir /tmp/bkp/

echo "Installing Percona Server 5.7"
yum -d 0 -y install http://www.percona.com/downloads/percona-release/redhat/0.1-4/percona-release-0.1-4.noarch.rpm
yum -d 0 -y install Percona-Server-server-57 Percona-Server-shared-57 Percona-Server-client-57 Percona-Server-shared-compat percona-toolkit percona-xtrabackup-24 vim-enhanced

echo "Installing orchestrator-agent"
cd /vagrant/orchestrator-agent
find -name "orchestrator-agent-*.rpm" | xargs yum -d 0 -y  install @

echo "Installing mydumper"
cd /vagrant/mydumper
find -name "mydumper-*.rpm" | xargs yum -d 0 -y  install @

echo "Copying my.cnf"
if [ "$customLogDir" = 1 ] ; then
  setenforce 0
  mkdir /var/lib/innodblog
  chown -R mysql:mysql /var/lib/innodblog
  if [[ -e "/vagrant/mysql_cnf/${HOSTNAME}-"$mysqlReplication"-customLogDir-my.cnf" ]]; then
    rm -f /etc/my.cnf
    cp /vagrant/mysql_cnf/${HOSTNAME}-"$mysqlReplication"-customLogDir-my.cnf /etc/my.cnf
  fi
else 
  if [[ -e "/vagrant/mysql_cnf/${HOSTNAME}-"$mysqlReplication"-my.cnf" ]]; then
    rm -f /etc/my.cnf
    cp /vagrant/mysql_cnf/${HOSTNAME}-"$mysqlReplication"-my.cnf /etc/my.cnf
  fi
fi


if [ "$orchAgentConfig" = "default" ] ; then
  echo "Copying default orchestrator-agent.conf.json"
  cp /vagrant/orchagent_cnf/orchestrator-agent.conf.json /etc/orchestrator-agent.conf.json
fi
if [ "$orchAgentConfig" = "backupusers" ] ; then
  echo "Copying BackupUsers-CompressBackup orchestrator-agent.conf.json" 
  cp /vagrant/orchagent_cnf/orchestrator-agent-BackupUsers-CompressBackup.conf.json /etc/orchestrator-agent.conf.json
fi
if [ "$orchAgentConfig" = "mydumperrows" ] ; then
  echo "Copying MyDumperRowsChunkSize orchestrator-agent.conf.json" 
  cp /vagrant/orchagent_cnf/orchestrator-agent-MyDumperRowsChunkSize.conf.json /etc/orchestrator-agent.conf.json
fi
if [ "$orchAgentConfig" = "backupolddatadir" ] ; then
  echo "Copying MySQLBackupOldDatadir orchestrator-agent.conf.json" 
  cp /vagrant/orchagent_cnf/orchestrator-agent-MySQLBackupOldDatadir.conf.json /etc/orchestrator-agent.conf.json
fi

echo "Starting MySQL"
service mysql start

echo "SLEEPING FOR 20 SECONDS to LET Percona Server start"
sleep 20s

echo "Updating password for root"
grep 'temporary password' /var/log/mysqld.log | grep -o ': .*$' | cut -c3-
PSWD=$(grep 'temporary password' /var/log/mysqld.log | grep -o ': .*$' | cut -c3-)
mysql -ss -uroot -p$PSWD -e "SET GLOBAL validate_password_policy=LOW;" --connect-expired-password

if [ "$HOSTNAME" = "orch-agent1" ] ; then
  mysql -ss -uroot -p$PSWD -e "ALTER USER 'root'@'localhost' IDENTIFIED BY 'privetserver'; FLUSH PRIVILEGES;" --connect-expired-password
  mysql -uroot -pprivetserver -e "grant all privileges on *.* to 'root'@'localhost'"

  echo "Creating orc_client_user and other"
  cat <<-EOF | mysql -uroot -pprivetserver
  USE mysql;
  CREATE USER IF NOT EXISTS 'orc_client_user'@'localhost' IDENTIFIED BY 'orc_client_password';
  CREATE USER IF NOT EXISTS 'user_1'@'localhost' IDENTIFIED BY 'privetserver';
  CREATE USER IF NOT EXISTS 'user_2'@'localhost' IDENTIFIED BY 'privetserver';
  GRANT ALL PRIVILEGES ON *.* TO 'orc_client_user'@'localhost' WITH GRANT OPTION;
  GRANT PROXY ON ''@'' TO 'orc_client_user'@'localhost' WITH GRANT OPTION;
  GRANT SELECT, DELETE ON *.* TO 'user_1'@'localhost';
  GRANT UPDATE ON *.* TO 'user_2'@'localhost';
  FLUSH PRIVILEGES;
EOF

  echo "Copying ~/.my.cnf"
  cp /vagrant/mysql_cnf/.my.cnf.orch-agent1 ~/.my.cnf

  echo "Removing validate_password plugin"
  mysql -uroot -pprivetserver -e "uninstall plugin validate_password;"
fi 

if [ "$HOSTNAME" = "orch-agent2" ] ; then
  mysql -ss -uroot -p$PSWD -e "ALTER USER 'root'@'localhost' IDENTIFIED BY 'privetserver1'; FLUSH PRIVILEGES;" --connect-expired-password
  mysql -uroot -pprivetserver1 -e "grant all privileges on *.* to 'root'@'localhost'"

  echo "Creating orc_client_user and other"
  cat <<-EOF | mysql -uroot -pprivetserver1
  USE mysql;
  CREATE USER IF NOT EXISTS 'orc_client_user'@'localhost' IDENTIFIED BY 'orc_client_password';
  CREATE USER IF NOT EXISTS 'user_1'@'localhost' IDENTIFIED BY 'privetserver1';
  CREATE USER IF NOT EXISTS 'user_2'@'localhost' IDENTIFIED BY 'privetserver1';
  CREATE USER IF NOT EXISTS 'slave_user_1'@'localhost' IDENTIFIED BY 'privetserver1';
  CREATE USER IF NOT EXISTS 'slave_user_2'@'localhost' IDENTIFIED BY 'privetserver1';
  GRANT ALL PRIVILEGES ON *.* TO 'orc_client_user'@'localhost' WITH GRANT OPTION;
  GRANT PROXY ON ''@'' TO 'orc_client_user'@'localhost' WITH GRANT OPTION;
  GRANT SELECT, DELETE ON *.* TO 'user_1'@'localhost';
  GRANT UPDATE ON *.* TO 'user_2'@'localhost';
  GRANT SELECT, DELETE ON *.* TO 'slave_user_1'@'localhost';
  GRANT UPDATE ON *.* TO 'slave_user_2'@'localhost';
  FLUSH PRIVILEGES;
EOF

  echo "Copying ~/.my.cnf"
  cp /vagrant/mysql_cnf/.my.cnf.orch-agent2 ~/.my.cnf

  echo "Removing validate_password plugin"
  mysql -uroot -pprivetserver1 -e "uninstall plugin validate_password;"
fi

echo "Updating /etc/hosts"
cat <<-EOF >> /etc/hosts
  192.168.58.201   orch-agent1
  192.168.58.202   orch-agent2
EOF

if [ "$HOSTNAME" = "orch-agent1" ] ; then
  echo "Creating databases"
  mysql -uroot -pprivetserver < /vagrant/mysql_db/sakila.sql
  mysql -uroot -pprivetserver < /vagrant/mysql_db/akila.sql
  mysql -uroot -pprivetserver < /vagrant/mysql_db/world.sql
fi

echo "Starting orchestrator-agent"
service orchestrator-agent start

sleep 5s

echo "Saving debug token"
mv /tmp/token.txt /vagrant/token_`hostname`.txt

if [[ -e /vagrant/db-post-install.sh ]]; then
  bash /vagrant/db-post-install.sh
fi

if [[ -e /vagrant/$HOSTNAME-post-install.sh ]]; then
  bash /vagrant/$HOSTNAME-post-install.sh
fi
