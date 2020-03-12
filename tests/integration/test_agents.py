import vagrant
import os
import pytest
import time
import json
import pprint

def test_mysqldump_positional(prepare_env, disable_gtid):
    sleep_interval_sec = 20
    seedID = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/Mysqldump/targetagent/sourceagent")
    print(seedID)
    assert seedID != ""
    while True:
        seed_status = {}
        seed_states = {}
        seed_status = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(seedID))))
        print("SEED STATUS:")
        pprint.pprint(seed_status)
        assert seed_status["Status"] != "Failed"
        seed_states = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(seedID))))
        print("SEED STATES FOR STAGE:")
        for seed_state in seed_states:
            if seed_state["Stage"] == seed_status["Stage"]:
                pprint.pprint(seed_state)
        if seed_status["Status"] == "Completed":
            break
        print("SLEEPING {} sec".format(sleep_interval_sec))
        time.sleep(sleep_interval_sec)

def test_mysqldump_gtid(prepare_env, enable_gtid):
    sleep_interval_sec = 20
    seedID = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/Mysqldump/targetagent/sourceagent")
    print(seedID)
    assert seedID != ""
    while True:
        seed_status = {}
        seed_states = {}
        seed_status = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(seedID))))
        print("SEED STATUS:")
        pprint.pprint(seed_status)
        assert seed_status["Status"] != "Failed"
        seed_states = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(seedID))))
        print("SEED STATES FOR STAGE:")
        for seed_state in seed_states:
            if seed_state["Stage"] == seed_status["Stage"]:
                pprint.pprint(seed_state)
        if seed_status["Status"] == "Completed":
            break
        print("SLEEPING {} sec".format(sleep_interval_sec))
        time.sleep(sleep_interval_sec)

def test_mydumper_positional(prepare_env, disable_gtid):
    sleep_interval_sec = 20
    seedID = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/Mydumper/targetagent/sourceagent")
    print(seedID)
    assert seedID != ""
    while True:
        seed_status = {}
        seed_states = {}
        seed_status = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(seedID))))
        print("SEED STATUS:")
        pprint.pprint(seed_status)
        assert seed_status["Status"] != "Failed"
        seed_states = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(seedID))))
        print("SEED STATES FOR STAGE:")
        for seed_state in seed_states:
            if seed_state["Stage"] == seed_status["Stage"]:
                pprint.pprint(seed_state)
        if seed_status["Status"] == "Completed":
            break
        print("SLEEPING {} sec".format(sleep_interval_sec))
        time.sleep(sleep_interval_sec)

def test_mydumper_gtid(prepare_env, enable_gtid):
    sleep_interval_sec = 20
    seedID = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/Mydumper/targetagent/sourceagent")
    print(seedID)
    assert seedID != ""
    while True:
        seed_status = {}
        seed_states = {}
        seed_status = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(seedID))))
        print("SEED STATUS:")
        pprint.pprint(seed_status)
        assert seed_status["Status"] != "Failed"
        seed_states = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(seedID))))
        print("SEED STATES FOR STAGE:")
        for seed_state in seed_states:
            if seed_state["Stage"] == seed_status["Stage"]:
                pprint.pprint(seed_state)
        if seed_status["Status"] == "Completed":
            break
        print("SLEEPING {} sec".format(sleep_interval_sec))
        time.sleep(sleep_interval_sec)

def test_xtrabackup_positional(prepare_env, disable_gtid):
    sleep_interval_sec = 20
    seedID = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/Xtrabackup/targetagent/sourceagent")
    print(seedID)
    assert seedID != ""
    while True:
        seed_status = {}
        seed_states = {}
        seed_status = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(seedID))))
        print("SEED STATUS:")
        pprint.pprint(seed_status)
        assert seed_status["Status"] != "Failed"
        seed_states = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(seedID))))
        print("SEED STATES FOR STAGE:")
        for seed_state in seed_states:
            if seed_state["Stage"] == seed_status["Stage"]:
                pprint.pprint(seed_state)
        if seed_status["Status"] == "Completed":
            break
        print("SLEEPING {} sec".format(sleep_interval_sec))
        time.sleep(sleep_interval_sec)

def test_xtrabackup_gtid(prepare_env, enable_gtid):
    sleep_interval_sec = 20
    seedID = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/Xtrabackup/targetagent/sourceagent")
    print(seedID)
    assert seedID != ""
    while True:
        seed_status = {}
        seed_states = {}
        seed_status = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(seedID))))
        print("SEED STATUS:")
        pprint.pprint(seed_status)
        assert seed_status["Status"] != "Failed"
        seed_states = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(seedID))))
        print("SEED STATES FOR STAGE:")
        for seed_state in seed_states:
            if seed_state["Stage"] == seed_status["Stage"]:
                pprint.pprint(seed_state)
        if seed_status["Status"] == "Completed":
            break
        print("SLEEPING {} sec".format(sleep_interval_sec))
        time.sleep(sleep_interval_sec)

def test_cloneplugin_positional(prepare_env, disable_gtid):
    version = prepare_env["orchestrator"].ssh(command="mysql -BNe 'select @@version'")
    if version[:1] == '5':
        pytest.skip("unsupported configuration")
    sleep_interval_sec = 20
    seedID = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/ClonePlugin/targetagent/sourceagent")
    print(seedID)
    assert seedID != ""
    while True:
        seed_status = {}
        seed_states = {}
        seed_status = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(seedID))))
        print("SEED STATUS:")
        pprint.pprint(seed_status)
        assert seed_status["Status"] != "Failed"
        seed_states = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(seedID))))
        print("SEED STATES FOR STAGE:")
        for seed_state in seed_states:
            if seed_state["Stage"] == seed_status["Stage"]:
                pprint.pprint(seed_state)
        if seed_status["Status"] == "Completed":
            break
        print("SLEEPING {} sec".format(sleep_interval_sec))
        time.sleep(sleep_interval_sec)

def test_cloneplugin_gtid(prepare_env, enable_gtid):
    version = prepare_env["orchestrator"].ssh(command="mysql -BNe 'select @@version'")
    if version[:1] == '5':
        pytest.skip("unsupported configuration")
    sleep_interval_sec = 20
    seedID = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/ClonePlugin/targetagent/sourceagent")
    print(seedID)
    assert seedID != ""
    while True:
        seed_status = {}
        seed_states = {}
        seed_status = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(seedID))))
        print("SEED STATUS:")
        pprint.pprint(seed_status)
        assert seed_status["Status"] != "Failed"
        seed_states = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(seedID))))
        print("SEED STATES FOR STAGE:")
        for seed_state in seed_states:
            if seed_state["Stage"] == seed_status["Stage"]:
                pprint.pprint(seed_state)
        if seed_status["Status"] == "Completed":
            break
        print("SLEEPING {} sec".format(sleep_interval_sec))
        time.sleep(sleep_interval_sec)

def test_lvm_positional(prepare_env, disable_gtid, reset_lvm):
    sleep_interval_sec = 20
    seedID = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/LVM/targetagent/sourceagent")
    print(seedID)
    assert seedID != ""
    while True:
        seed_status = {}
        seed_states = {}
        seed_status = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(seedID))))
        print("SEED STATUS:")
        pprint.pprint(seed_status)
        assert seed_status["Status"] != "Failed"
        seed_states = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(seedID))))
        print("SEED STATES FOR STAGE:")
        for seed_state in seed_states:
            if seed_state["Stage"] == seed_status["Stage"]:
                pprint.pprint(seed_state)
        if seed_status["Status"] == "Completed":
            break
        print("SLEEPING {} sec".format(sleep_interval_sec))
        time.sleep(sleep_interval_sec)

def test_lvm_gtid(prepare_env, enable_gtid, reset_lvm):
    sleep_interval_sec = 20
    seedID = prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed/LVM/targetagent/sourceagent")
    print(seedID)
    assert seedID != ""
    while True:
        seed_status = {}
        seed_states = {}
        seed_status = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-details/{}".format(int(seedID))))
        print("SEED STATUS:")
        pprint.pprint(seed_status)
        assert seed_status["Status"] != "Failed"
        seed_states = json.loads(prepare_env["orchestrator"].ssh(command="curl -X GET http://localhost:3000/api/agent-seed-states/{}".format(int(seedID))))
        print("SEED STATES FOR STAGE:")
        for seed_state in seed_states:
            if seed_state["Stage"] == seed_status["Stage"]:
                pprint.pprint(seed_state)
        if seed_status["Status"] == "Completed":
            break
        print("SLEEPING {} sec".format(sleep_interval_sec))
        time.sleep(sleep_interval_sec)

