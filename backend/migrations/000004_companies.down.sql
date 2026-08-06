DROP TABLE IF EXISTS company_documents;
DROP TABLE IF EXISTS company_members;
ALTER TABLE IF EXISTS user_roles DROP CONSTRAINT IF EXISTS fk_user_roles_company;
DROP TABLE IF EXISTS companies;
