# vuln demo

A tiny synthetic project whose second commit introduces a SQL injection,
so AurumCode's security pass (`aurumcode review --base HEAD~1
--seguranca`, card AUR-435) has a real vulnerability to find in the
`HEAD~1..HEAD` diff. Nothing here is a real application, credential, or
database.
