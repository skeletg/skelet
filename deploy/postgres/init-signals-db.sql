-- Small profile puts both schemas on one Postgres cluster - the commerce db comes from POSTGRES_DB, this adds the
-- Signals one beside it. Runs once, on first init of an empty data volume (postgres ignores it afterwards)
CREATE DATABASE skelet_signals OWNER skelet;
