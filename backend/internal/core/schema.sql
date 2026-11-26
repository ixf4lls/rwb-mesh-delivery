-- Initial, idempotent PostgreSQL schema. Apply with mesh migrate.
CREATE TABLE IF NOT EXISTS mesh_records(scope text NOT NULL,kind text NOT NULL,id text NOT NULL,data jsonb NOT NULL,PRIMARY KEY(scope,kind,id));
CREATE INDEX IF NOT EXISTS mesh_records_data ON mesh_records USING gin(data jsonb_path_ops);
CREATE TABLE IF NOT EXISTS mesh_idempotency(subject text NOT NULL,scope text NOT NULL,method text NOT NULL,path text NOT NULL,key text NOT NULL,hash text NOT NULL,status int NOT NULL,body jsonb NOT NULL,PRIMARY KEY(subject,scope,method,path,key));
