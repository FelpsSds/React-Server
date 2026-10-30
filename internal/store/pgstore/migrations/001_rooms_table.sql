CREATE TABLE IF NOT EXISTS rooms (
    "id" uuid PRIMARY KEY NOT NULL DEFAULT gen_random_uuid(),
    "name" varchar(255) NOT NULL,
    
);

---- create above / drop below ----

DROP TABLE IF EXISTS rooms;

-- Write your migrate down statements here. If this migration is irreversible
-- Then delete the separator line above.
