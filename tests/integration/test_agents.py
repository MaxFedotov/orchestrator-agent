import vagrant
import os
import pytest

def test_orchestrator(prepare_env):
    stdout = prepare_env["orchestrator"].ssh(command="mysql -BNe 'show databases;'")
    print(stdout)
    assert stdout != ""

def test_sourceAgent(prepare_env):
    stdout = prepare_env["sourceagent"].ssh(command="mysql -BNe 'show databases;'")
    print(stdout)
    assert stdout != ""

def test_targetAgent(prepare_env):
    stdout = prepare_env["targetagent"].ssh(command="mysql -BNe 'show databases;'")
    print(stdout)
    assert stdout != ""


