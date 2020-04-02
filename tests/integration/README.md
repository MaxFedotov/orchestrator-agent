## Orchestrator-agent integraton tests

This directory contains tests that involve 2 vagrant instances with Orchestrator-agent\MySQL (one for source host, one for target) and 1 vagrant host with Orchestrator\MySQL.  
They tests different seed methods with different replication configuration (GTID of positional replication) on MySQL 5.7 or 8.0 versions (this parameter is configurable before a test run).  

Prerequisites:
* Python > 3
* [Vagrant](https://www.vagrantup.com/downloads.html)
* [Virtualbox](https://www.virtualbox.org/wiki/Downloads)
* [Pip](https://pypi.python.org/pypi/pip)
* [Goreleaser](https://goreleaser.com/install/)
* [fpm] (https://fpm.readthedocs.io/en/latest/installing.html)
* rpm

Tests use pytest framework and python bindings for vagrant in order to spawn vagrant VMs.  

You can install them using pip and following command:  
`pip install -r requirements.txt`

Tests will automatically download specific prepared vagrant boxes from vagrant cloud. Size of box is about 1.2Gb.  

Each test is located in a sepated folder, starting with `test_`  

Run tests with the `pytest` command. To select which tests to run, use `pytest -k <test_name_pattern>`  

If you want to enable verbose logging during test ran, use `pytest -v`  

Tests have following options, which you can pass to `pytest` command:
* --mysql_version (57|80) - on which version of MySQL to run tests, default is 57. Please note, that clone_plugin method tests will be ran only using 80 version
* --orch_repo - from which repository build Orchestrator binaries, default is https://github.com/openark/orchestrator.git
* --orch_branch - from which branch of repository build Orchestrator binaries, default is master
* --only-update-agents - when running tests, do not recreate Orchestator vagrant box. Only update Orchestrator-agent vagrant boxes, default is False
* --no-destroy-vagrant-boxes - do not destroy vagrant boxes after tests will be completed

How to SSH to vagrant box:
* change directory to a folder, where vagrant file with mysql version you specified when starting tests is located (integration/vagrant/vagrantfiles/mysql_57 or integration/vagrant/vagrantfiles/mysql_80)
* `EXPORT hostname={$name_of_the_VM_you_want_to_ssh}`, it can be one of the orchestrator|targetagent|sourceagent
* execute `vagrant ssh` command

On MacOS you can add following to `/etc/sudoers` in order to prevent asking every time for root password when starting vagrant:
```console
Cmnd_Alias VAGRANT_EXPORTS_ADD = /usr/bin/tee -a /etc/exports
Cmnd_Alias VAGRANT_NFSD = /sbin/nfsd restart
Cmnd_Alias VAGRANT_EXPORTS_REMOVE = /usr/bin/sed -E -e /*/ d -ibak /etc/exports
%admin ALL=(root) NOPASSWD: VAGRANT_EXPORTS_ADD, VAGRANT_NFSD, VAGRANT_EXPORTS_REMOVE
```