package webhook

// GitHub webhook fixtures for testing

const pullRequestOpenedPayload = `{
  "action": "opened",
  "number": 42,
  "pull_request": {
    "number": 42,
    "state": "open",
    "title": "Add new feature",
    "user": {
      "login": "contributor"
    },
    "head": {
      "ref": "feature-branch",
      "sha": "abc123def456"
    },
    "base": {
      "ref": "main",
      "sha": "def456abc789"
    }
  },
  "repository": {
    "full_name": "owner/repo",
    "name": "repo",
    "owner": {
      "login": "owner"
    }
  }
}`

const pullRequestSynchronizePayload = `{
  "action": "synchronize",
  "number": 42,
  "pull_request": {
    "number": 42,
    "state": "open",
    "title": "Add new feature",
    "head": {
      "ref": "feature-branch",
      "sha": "xyz789new"
    },
    "base": {
      "ref": "main",
      "sha": "def456abc789"
    }
  },
  "repository": {
    "full_name": "owner/repo"
  }
}`

const pushPayload = `{
  "ref": "refs/heads/main",
  "before": "abc123",
  "after": "def456",
  "repository": {
    "full_name": "owner/repo",
    "name": "repo",
    "owner": {
      "login": "owner"
    }
  },
  "pusher": {
    "name": "pusher-user"
  },
  "commits": [
    {
      "id": "def456",
      "message": "Update README",
      "author": {
        "name": "Author Name"
      }
    }
  ]
}`

const pushNonMainPayload = `{
  "ref": "refs/heads/develop",
  "repository": {
    "full_name": "owner/repo"
  }
}`

const pullRequestClosedPayload = `{
  "action": "closed",
  "number": 42,
  "pull_request": {
    "number": 42,
    "merged": true
  },
  "repository": {
    "full_name": "owner/repo"
  }
}`
