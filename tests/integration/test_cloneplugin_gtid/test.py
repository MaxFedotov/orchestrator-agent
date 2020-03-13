import pytest
import pprint
from helpers.waiter import OrchestratorWaiter

def test_cloneplugin_gtid(prepare_env, enable_gtid):
    version = prepare_env["orchestrator"].ssh(command="mysql -BNe 'select @@version'")
    if version[:1] == '5':
        print("Unsupported configuration. ClonePlugin supported only for MySQL 8")
        pytest.skip()
    seed_id = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/ClonePlugin/targetagent/sourceagent")
    assert seed_id != ""
    print("******* STARTING SEED {}*******".format(seed_id))
    waiter = OrchestratorWaiter(seed_id, prepare_env["orchestrator"])
    waiter.wait()