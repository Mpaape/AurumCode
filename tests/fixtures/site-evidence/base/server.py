from app import find_user


def handle(request):
    return find_user(request["id"])
