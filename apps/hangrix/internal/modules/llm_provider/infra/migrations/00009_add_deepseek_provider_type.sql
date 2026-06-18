-- +goose Up

-- Allow DeepSeek to be registered as a first-class provider type while still
-- using the same llm_providers table and routing model.
-- +goose StatementBegin
DO $$
DECLARE
    constraint_name text;
BEGIN
    SELECT con.conname INTO constraint_name
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    WHERE rel.relname = 'llm_providers'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) LIKE 'CHECK%type%';

    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE llm_providers DROP CONSTRAINT %I', constraint_name);
    END IF;

    EXECUTE $alter$ALTER TABLE llm_providers ADD CONSTRAINT llm_providers_type_check
        CHECK (type IN ('openai', 'anthropic', 'openai-compat', 'deepseek', 'mock'))$alter$;
END;
$$;
-- +goose StatementEnd

-- +goose Down

-- Revert to the provider type set before DeepSeek became first-class.
-- +goose StatementBegin
DO $$
DECLARE
    constraint_name text;
BEGIN
    SELECT con.conname INTO constraint_name
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    WHERE rel.relname = 'llm_providers'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) LIKE 'CHECK%type%';

    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE llm_providers DROP CONSTRAINT %I', constraint_name);
    END IF;

    EXECUTE $alter$ALTER TABLE llm_providers ADD CONSTRAINT llm_providers_type_check
        CHECK (type IN ('openai', 'anthropic', 'openai-compat', 'mock'))$alter$;
END;
$$;
-- +goose StatementEnd
