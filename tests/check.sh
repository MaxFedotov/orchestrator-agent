vagrant ssh orch-agent2 -- -t "sudo mysql -e 'SHOW SLAVE STATUS \G'"
vagrant ssh orch-agent2 -- -t "sudo mysql -e 'SHOW DATABASES;'"
vagrant ssh orch-agent1 -- -t "sudo mysql -e 'use sakila; update actor set actor_id=0 where actor_id=1;'"
vagrant ssh orch-agent2 -- -t "sudo mysql -e 'SHOW SLAVE STATUS \G'"

# for --orchagent-config=backupusers
echo "TEST USER FROM SLAVE"
vagrant ssh orch-agent2 -- -t "sudo mysql -uuser_1 -e 'use sakila; SELECT 1;'"
echo "TEST USER FROM SLAVE"
vagrant ssh orch-agent2 -- -t "sudo mysql -uslave_user_1 -e 'use sakila; SELECT 1;'"
echo "TEST USER FROM MASTER. SHOULD RETURN ERROR IN CASE OF PARTIAL BACKUP"
vagrant ssh orch-agent2 -- -t "sudo mysql -uuser_2 -pprivetserver -e 'use sakila; SELECT 1;'"
# this should return error
echo "TEST USER WHICH SHOULD NOT EXIST"
vagrant ssh orch-agent2 -- -t "sudo mysql -uslave_user_2 -e 'use sakila; SELECT 1;'"