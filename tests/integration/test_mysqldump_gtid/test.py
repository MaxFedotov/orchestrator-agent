import pytest
import pprint
from helpers.waiter import OrchestratorWaiter

def test_mysqldump_gtid(prepare_env, enable_gtid):
    seed_id = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/Mysqldump/targetagent/sourceagent")
    assert seed_id != ""
    print("******* STARTING SEED {}*******".format(seed_id))
    waiter = OrchestratorWaiter(seed_id, prepare_env["orchestrator"])
    waiter.wait()