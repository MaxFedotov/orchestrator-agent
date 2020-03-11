flush tables with read lock;
SYSTEM mysql -ANe "SHOW MASTER STATUS"| awk '{print "File:"$1"\n""Position:"$2"\n""Executed_Gtid_Set:"$3}' > /var/lib/mysql/metadata && chown mysql:mysql /var/lib/mysql/metadata
SYSTEM sudo bash -c 'lvcreate -l20%FREE -s -n mysql-backup_$(date +%s) /dev/mysql_vg/mysql_lv'
unlock tables;