sed -i -e "s/.*sourceToken.*/  sourceToken: $(cat vagrant/orch-agent1.token.txt)/" common.yaml
sed -i -e "s/.*targetToken.*/  targetToken: $(cat vagrant/orch-agent2.token.txt)/" common.yaml
rm -rf common.yaml-e