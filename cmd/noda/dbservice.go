package main

// isDatabaseService reports whether a service config uses the Postgres/db
// plugin. The db plugin's Name() is "postgres" and its Prefix() is "db";
// scaffolders and hand-written configs use either, so both are accepted.
func isDatabaseService(svc map[string]any) bool {
	p, _ := svc["plugin"].(string)
	return p == "db" || p == "postgres"
}
