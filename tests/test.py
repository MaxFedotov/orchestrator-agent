import requests
import urllib


def save_port(response):
    if response.text == "":
        raise ValueError("Response is empty")
    return {"port" : response.text}

def save_backupFolder(response):
    if response.text == "":
        raise ValueError("Response is empty")
    return {"backupFolder" : urllib.parse.quote_plus(response.text.strip('"'))}

def validate_response(response):
    if response.text == "":
        raise ValueError("Response is empty")
    return {"response" : response.text}

