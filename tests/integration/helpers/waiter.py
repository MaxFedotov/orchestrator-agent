import time
import pprint
import json
import pytest

class TimeoutException(Exception):
    pass

class OrchestratorWaiter:
    def __init__(self,seed_id,vagrant_box, interval=20, timeout=600):
        self.interval = interval
        self.timeout = timeout
        self.seed_id = seed_id
        self.vagrant_box = vagrant_box


    def wait(self):
        start_time = time.time()

        while (time.time() - start_time) <= self.timeout:
            seed_status = json.loads(self.vagrant_box.ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(self.seed_id))))
            print("******* SEED STATUS *******")
            pprint.pprint(seed_status)
            assert seed_status["Status"] != "Failed"
            seed_states = json.loads(self.vagrant_box.ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(self.seed_id))))
            if len(seed_states) > 0:
                print("******* LAST SEED STATE *******")
                pprint.pprint(seed_states[0])
            if seed_status["Status"] == "Completed":
                return
            print("******* SLEEPING FOR {} SEC *******".format(self.interval))
            time.sleep(self.interval)

        raise TimeoutException("Timed out waiting for seedID %s." % self.seed_id)