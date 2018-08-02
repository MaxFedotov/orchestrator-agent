sed -i -e "s/.*sourceToken.*/  sourceToken: $(cat vagrant/token_orch-agent1.txt)/" common.yaml
sed -i -e "s/.*targetToken.*/  targetToken: $(cat vagrant/token_orch-agent2.txt)/" common.yaml
rm -rf common.yaml-e