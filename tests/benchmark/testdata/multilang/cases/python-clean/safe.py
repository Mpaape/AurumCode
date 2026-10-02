import subprocess

def archive(name):
    subprocess.run(["tar", "czf", "out.tgz", name], check=True)
