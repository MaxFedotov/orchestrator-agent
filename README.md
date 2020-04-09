orchestrator-agent
==================

MySQL topology and seeding agent (daemon)

**orchestrator-agent** is a sub-project of [orchestrator](https://github.com/openark/orchestrator).
It is a service that runs on MySQL hosts and communicates with *orchestrator*.

**orchestrator-agent** is capable of seeding new replicas using different seed methods, providing operating system, file system and LVM information to *orchestrator*, managing MySQL service as well as invoke certain commands and scripts.

##### Generic functionality offered by **orchestrator-agent**:

- Detection of the MySQL service, starting and stopping (start/stop/status commands provided via configuration)
- Detection of MySQL port, data directory, databases
- Calculation of disk usage on data directory mount point, OS and RAM size
- Tailing the error log file
- Discovery (the mere existence of the *orchestrator-agent* service on a host may suggest the existence or need of existence of a MySQL service)
 
##### Specialized functionality offered by **orchestrator-agent**:
- Seeding new slaves using different seed methods (LVM, Xtrabackup, Mydumper, Mysqldump, Clone plugin)
- Detection of LVM snapshots on MySQL host (snapshots that are MySQL specific)
- Creation of new snapshots
- Mounting/umounting of LVM snapshots
- Detection of DC-local and DC-agnostic snapshots available for a given cluster

##### Supported seed methods
- Mysqldump
- Mydumper
- Xtrabackup
- LVM
- Clone plugin

### The orchestrator & orchestrator-agent architecture

**orchestrator** is a standalone, centralized service/command line tool. When acting as a service, it provides with web API
and web interface to allow replication topology refactoring, long query control, and more.

Coupled with **orchestrator-agent**, **orchestrator** is further able to assist in seeding new/corrupt servers. 
**orchestrator-agent** does not initiate anything by itself, but is in fact controlled by **orchestrator**.

When started, **orchestrator-agent** chooses a random, secret *token* and attempts to connect to the centralized **orchestrator**
service API (location configurable). It then registers at the **orchestrator** service with its secret token. 

**orchestrator-agent** then serves via HTTP API, and for all but the simplest commands requires the secret token.

At this point **orchestrator** becomes the major player; having multiple **orchestrator-agent** registered it is able to
coordinate operations such as seeding, connecting seeded hosts as slaves, snapshot mounting, space cleanup.

### Configuration

_Orchestrator-agent_ uses a configuration file, located in `/etc/orchestrator-agent.conf`  
Configuration file uses TOML format and is divided into following sections:

##### [common]
* `port` (int), Port to listen on. Default: 3002
* `seed-port` (int), Port used to transfer seed data between agents using socat. Used with Xtrabackup and LVM seed methods. Default: 21234
* `poll-interval` (time.Duration), Interval for heartbeat checks to Orchestrator. Default: "1m0s"
* `resubmit-agent-interval` (time.Duration), Interval for resubmitting agent data to Orchestrator. Default: "60m0s"
* `http-auth-user` (string), Username for HTTP Basic authentication (blank disables authentication). Default: ""
* `http-auth-password` (string), Password for HTTP Basic authentication. Default: ""
* `http-timeout` (time.Duration), HTTP GET request timeout (when connecting to _orchestrator_). Default: "10s"
* `use-ssl` (bool), If `true` then serving via `https` protocol. Default: false
* `use-mutual-tls` (bool), If `true`, service will serve `https` only. Default: false
* `ssl-skip-verify` (bool), When connecting to **orchestrator** via SSL, whether to ignore certification error. Default: false
* `ssl-cert-file` (string), When serving via `https`, location of SSL certification file. Default: ""
* `ssl-private-key-file` (string), When serving via `https`, location of SSL private key file. Default: ""
* `ssl-ca-file` (string), When serving via `https`, name of SSL certificate authority file. Default: ""
* `ssl-valid-ous` ([]string), When serving via `https`, list of valid OUs that should be allowed for mutual TLS verification. Default: []
* `status-ou-verify` (bool), If true, try to verify OUs when Mutual TLS is on. Default: false
* `token-hint-file` (string), If defined, token will be stored in this file. Default: ""
* `token-http-header` (string), If defined, name of HTTP header where token is presented (as alternative to query param). Default: ""
* `exec-with-sudo` (bool), If true, run os commands that need privileged access with sudo. Default: false
* `sudo-user` (string), If `exec-with-sudo`, use this user for running commands (sudo -u `sudo-user`). Default: ""
* `backup-dir` (string), **Required** Directory for storing backup data during seed (or directory to mount snapshots if LVM seed method is used). Default: ""
* `status-endpoint` (string), The endpoint for the agent status check. Default: "/api/status"
* `status-bad-seconds` (time.Duration), Report non-200 on a status check if we've failed to communicate with the main server in this number of seconds. Default: "300s"

##### [orchestrator]
* `url` (string), **Required** URL of your **orchestrator** daemon. Default: ""
* `agents-port` (uint), Port configured in **orchestrator** for agent connections. Default: 3001

##### [logging]
* `file` (string), File to store orchestrator-agent logs. Default: "/var/log/orchestrator-agent.log"
* `level` (string), Log level. Possible values are: debug, info, error, warn. Default: "info"

##### [mysql]
* `port` (int), Port used by MySQL. Default: 3306
* `user` (string), **Required** User to connect to MySQL. Default: "" (See orchestrator-agent MySQL grants)
* `password` (string), **Required** Password for MySQL user. Default: ""
* `service-status-command` (string), Command which stops the MySQL service. Default: "systemctl check mysqld"
* `service-start-command` (string), Command which starts the MySQL service. Default: "systemctl start mysqld"
* `service-stop-command` (string), Command which stops the MySQL service. Default: "systemctl stop mysqld"

##### [custom-commands]
Under this section goes a list of your custom commands, that will be availiable for executing on agent using **orchestrator** UI.  
The commands are written using following notation:  
[custom-commands.command_name_1]  
cmd = "echo hello"  
[custom-commands.command_name_2]  
cmd = "echo hello2"  
  
One special command is a post-seed command. It will be executed after every successful seed operation on both agents, e.g:  
[custom-commands.post-seed-command]  
cmd = "/tmp/post_seed.py"  
This will execute post_seed.py script on both agents after successful seed operation  

##### [mysqldump]
* `enabled` (bool), If true, mysqldump seed method will be availiable for seeds. Default: true
* `mysqldump-addtional-opts` ([]string), Array of additional command-line options to pass to mysqldump command. Default: ["--single-transaction", "--quick", "--routines", "--events", "--triggers", "--hex-blob"]. **If you want to add additional options, it's better to copy defaults and add your new options to the end of array.** There is also a list of reserved options, that cannot be changed or set using this configuration parameter - ["--host", "--user", "--port", "--password", "--master-data", "--all-databases"]

##### [mydumper]
* `enabled` (bool), If true, mydumper seed method will be availiable for seeds (if it is installed). Default: false
* `mydumper-addtional-opts` ([]string), Array of additional command-line options to pass to mydumper command. Default: ["--routines", "--events", "--triggers"]. **If you want to add additional options, it's better to copy defaults and add your new options to the end of array.** There is also a list of reserved options, that cannot be changed or set using this configuration parameter - ["--host", "--user", "--port", "--password", "--outputdir", "--overwrite-tables", "--directory", "--no-backup-locks"]
* `myloader-additional-opts` ([]string), Array of additional command-line options to pass to mydumper command. Default: []. There is also a list of reserved options, that cannot be changed or set using this configuration parameter - ["--host", "--user", "--port", "--password", "--outputdir", "--overwrite-tables", "--directory", "--no-backup-locks"]

##### [xtrabackup]
* `enabled` (bool), If true, xtrabackup seed method will be availiable for seeds (if it is installed). Default: false
* `xtrabackup-addtional-opts` ([]string), Array of additional command-line options to pass to xtrabackup command. Default: []. There is also a list of reserved options, that cannot be changed or set using this configuration parameter - ["--host", "--user", "--port", "--password", "--target-dir", "--decompress", "--backup", "--stream", "--prepare"]
* `socat-use-ssl` (bool), If `true` then ssl will be used when transmitting seed data using socat between agents. Default: false
* `socat-ssl-cert-file` (string), When using `socat-use-ssl`, location of SSL certification file. Default: ""
* `socat-ssl-cat-file` (string), When using `socat-use-ssl`, name of SSL certificate authority file. Default: ""
* `socat-ssl-skip-verify` (bool), When using `socat-use-ssl`, whether to ignore certification error. Default: false

##### [lvm]
* `enabled` (bool), If true, lvm seed method will be availiable for seeds (if lvm is configured). Default: false
* `create-snapshot-command` (string), Command which creates new LVM snapshot of MySQL data. Default: ""
* `create-new-snapshot-for-seed ` (bool), If true new snapshot will be created during seed process. **`create-snapshot-command` is required**. Default: false
* `available-local-snapshot-hosts-command` (string), Command which returns list of hosts in local DC on which recent snapshots are available. Default: ""
* `available-snapshot-hosts-command` (string), Command which returns list of hosts in all DCs on which recent snapshots are available. Default: ""
* `snapshot-volumes-filter` (string), free text which identifies MySQL data snapshots (as opposed to other, unrelated snapshots). Default: ""
* `socat-use-ssl` (bool), If `true` then ssl will be used when transmitting seed data using socat between agents. Default: false
* `socat-ssl-cert-file` (string), When using `socat-use-ssl`, location of SSL certification file. Default: ""
* `socat-ssl-cat-file` (string), When using `socat-use-ssl`, name of SSL certificate authority file. Default: ""
* `socat-ssl-skip-verify` (bool), When using `socat-use-ssl`, whether to ignore certification error. Default: false

##### [clone_plugin]
* `enabled` (bool), If true, lvm seed method will be availiable for seeds (if it is [installed in MySQL](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-installation.html)). Default: false 
* `clone-autotune-concurrency` (bool), [See docs for description](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-options-variables.html#sysvar_clone_autotune_concurrency). Default: true
* `clone-buffer-size` (int64), [See docs for description](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-options-variables.html#sysvar_clone_buffer_size). Default: 4194304
* `clone-ddl-timeout` (int64), [See docs for description](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-options-variables.html#sysvar_clone_ddl_timeout). Default: 300
* `clone-enable-compression` (bool), [See docs for description](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-options-variables.html#sysvar_clone_enable_compression). Default: false
* `clone-max-concurrency` (int), [See docs for description](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-options-variables.html#sysvar_clone_max_concurrency). Default: 16
* `clone-max-data-bandwidth` (int), [See docs for description](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-options-variables.html#sysvar_clone_max_data_bandwidth). Default: 0
* `clone-max-network-bandwidth` (int), [See docs for description](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-options-variables.html#sysvar_clone_max_network_bandwidth). Default: 0
* `clone-ssl-ca` (string), [See docs for description](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-options-variables.html#sysvar_clone_ssl_ca). Default: ""
* `clone-ssl-cert` (string), [See docs for description](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-options-variables.html#sysvar_clone_ssl_cert). Default: ""
* `clone-ssl-key` (string), [See docs for description](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-options-variables.html#sysvar_clone_ssl_key). Default: ""  

Example configuration file can be found in **/conf/orchestrator-agent.conf** directory.

### Example additonal opts for different seed methods

#### Mysqldump
* Enable compression:
```toml
[mysqldump]
enabled = true
mysqldump-addtional-opts = ["--single-transaction", "--quick", "--routines", "--events", "--triggers", "--hex-blob", "--compress"]
```

#### Mydumper
* Use 6 threads for mydumper/myloader and split table into chunks of 100000 rows:
```toml
[mydumper]
enabled = true
mydumper-addtional-opts = ["--triggers", "--events", "--routines", "--compress", "--threads 6", "--rows 100000"]
myloader-additional-opts = ["--threads 6"]
```

#### Xtrabackup
* Use 4 threads for backup and enable compression (requires **qpress** installed): 
```toml
[xtrabackup]
enabled = true
xtrabackup-addtional-opts = ["--parallel=4", "--compress"]
```

### Notes on LVM seed method
* For LVM seed method `backup-dir` is a path, where snapshots will be mounted
* If your source server is MySQL slave you should use `skip-slave-start` option on your target server
* When creating your own snapshots or if using `create-snapshot-command` option you should create a file called `metadata` in MySQL datadir with following content:  
```
File:mysql-bin.000009
Position:701
Executed_Gtid_Set:5c2bd8fc-5ee3-11ea-adf4-5254008afee6:1-741
```
This file will be used later by **orchestrator** and **orchestrator-agent** in order to connect your target server as slave.  
As a very simple example, you can use following script for creating snaphosts:  
```sql
FLUSH TABLES WITH READ LOCK;
SYSTEM mysql -ANe "SHOW MASTER STATUS"| awk '{print "File:"$1"\n""Position:"$2"\n""Executed_Gtid_Set:"$3}' > /var/lib/mysql/metadata && chown mysql:mysql /var/lib/mysql/metadata
SYSTEM sudo bash -c 'lvcreate -l20%FREE -s -n mysql-backup_$(date +%s) /dev/mysql_vg/mysql_lv'
UNLOCK TABLES;
```
Add permissions to `mysql` system user for lvcreate command by adding following lines to `/etc/sudoers.d/mysql`
```
mysql ALL = (root) NOPASSWD: /usr/bin/lvcreate *
```
Put this script to `/tmp/create_snapshot.sql` file and use following configuration
```toml
[lvm]
enabled = true
create-snapshot-command = "cat /tmp/create_snapshot.sql | mysql"
create-new-snapshot-for-seed = true
```

### Permissions for orchestrator-agent

#### OS user
By default orchestrator-agent runs under `mysql` user account, because LVM\Xtrabackup seed methods requires adding\removing data from MySQL datadir.  
In order to be able to control MySQL service status (and that is necessary for these seed methods), you can to do following:  
* In /etc/orchestrator.conf set following options in `mysql` section:
```toml
[mysql]
service-status-command = "sudo systemctl check mysqld"
service-start-command = "sudo systemctl start mysqld"
service-stop-command = "sudo systemctl stop mysqld"
```
* Create file `/etc/sudoers.d/mysql` with following options, that will grant `mysql` user permission to control MySQL service
```
mysql ALL = (root) NOPASSWD: /usr/bin/systemctl * mysqld
```

#### MySQL user
```sql
CREATE USER 'orchestrator-agent'@'%' identified WITH mysql_native_password by 'orchestrator-agent-pwd';
```
```sql
GRANT ALTER, ALTER ROUTINE, CREATE, CREATE ROUTINE, SHOW VIEW, CREATE TABLESPACE, CREATE TEMPORARY TABLES, CREATE VIEW, DELETE, DROP, EVENT, INDEX, INSERT, LOCK TABLES, PROCESS, REFERENCES, RELOAD, REPLICATION CLIENT, SELECT, SUPER, TRIGGER, UPDATE on *.* to 'orchestrator-agent'@'%';
```

If you are using MySQL 8 and want to use clone plugin, you will need to add additional grants:
```sql
GRANT BACKUP_ADMIN, CLONE_ADMIN, SYSTEM_USER on *.* to 'orchestrator-agent'@'%';
```

### Necessary matching configuration on the Orchestrator Server side

If you initially deployed orchestrator with a minimally working configuration, you will need to make some changes on the server side to prepare it for newly deployed agents. The configuration lines needed on the server side to support agents are

* `ServeAgentsHttp` (bool), Must be set to `true` to get the orchestrator server listening for agents
* `AgentsServerPort`(String), The port on which the server should listen to agents. Shoult match the port you define for agents in `agents-port`.

### Requirements:

- Linux, 64bit. Tested on CentOS 7
- MySQL 5.6+
- For xtrabackup seed method you need [Xtrabackup](https://www.percona.com/software/mysql-database/percona-xtrabackup) installed
- For mydumper seed method you need [Mydumper](https://github.com/maxbube/mydumper) installed
- For clone plugin seed method you need MySQL 8 and [Clone plugin](https://dev.mysql.com/doc/refman/8.0/en/clone-plugin-installation.html) installed
- For LVM seed method you need configured LVM, free space in volume group, if snapshot functionality is required
- **orchestrator-agent** assumes a single MySQL running on the machine

### Bulding
Orchestrator-agent uses [goreleaser](https://goreleaser.com/) and custom wrapper for [fpm](https://github.com/jordansissel/fpm) to build packages.
In order to build it, install them and use following command
```
goreleaser --rm-dist --snapshot
```
This build binaries and create all packages in `dist` folder

### Extending orchestrator-agent

Yes please. **orchestrator-agent** is open to pull-requests

Authored by [Shlomi Noach](https://github.com/shlomi-noach) at [GitHub](http://github.com). Previously at [Booking.com](http://booking.com) and [Outbrain](http://outbrain.com)

Additonal seed methods added by [Max Fedotov](https://github.com/MaxFedotov) at [Wargaming.net](wargaming.net)
