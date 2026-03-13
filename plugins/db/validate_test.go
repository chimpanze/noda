package db

import (
	"testing"
)

func TestValidateIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"simple name", "users", false},
		{"underscore prefix", "_id", false},
		{"with numbers", "table1", false},
		{"dotted schema.table", "public.users", false},
		{"dotted schema.table.column", "public.users.id", false},
		{"uppercase", "Users", false},
		{"mixed case", "myTable", false},

		{"empty string", "", true},
		{"starts with number", "1table", true},
		{"contains space", "my table", true},
		{"contains dash", "my-table", true},
		{"SQL injection semicolon", "users; DROP TABLE users", true},
		{"SQL injection comment", "users--", true},
		{"contains parens", "count(*)", true},
		{"contains quotes", "users'", true},
		{"contains double quotes", `users"`, true},
		{"contains equals", "a=b", true},
		{"starts with dot", ".users", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdentifier(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIdentifier(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateOrderClause(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"single column", "name", false},
		{"column ASC", "name ASC", false},
		{"column DESC", "name DESC", false},
		{"column asc lowercase", "name asc", false},
		{"multiple columns", "name ASC, age DESC", false},
		{"dotted column", "users.name ASC", false},
		{"three columns", "a, b DESC, c ASC", false},

		{"empty string", "", true},
		{"trailing comma", "name,", true},
		{"injection in order", "name; DROP TABLE users", true},
		{"random word direction", "name ASCENDING", true},
		{"subquery attempt", "name,(SELECT 1)", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOrderClause(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOrderClause(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateJoinType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"INNER", "INNER", false},
		{"LEFT", "LEFT", false},
		{"RIGHT", "RIGHT", false},
		{"FULL", "FULL", false},
		{"CROSS", "CROSS", false},
		{"lowercase inner", "inner", false},
		{"mixed case Left", "Left", false},

		{"empty", "", true},
		{"NATURAL", "NATURAL", true},
		{"LEFT OUTER", "LEFT OUTER", true},
		{"injection", "INNER; DROP", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJoinType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateJoinType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSQLFragment(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"simple expression", "age > 18", false},
		{"column reference", "users.active = true", false},
		{"empty string", "", false},
		{"column named created_at", "a.created_at = b.created_at", false},
		{"column named updated_at", "updated_at IS NOT NULL", false},
		{"column with select substring", "user_selection = 'yes'", false},
		{"valid ON clause", "a.id = b.user_id AND a.active = true", false},

		{"semicolon", "1; DROP TABLE users", true},
		{"line comment", "1 -- comment", true},
		{"block comment", "1 /* comment */", true},
		{"just semicolon", ";", true},
		{"just comment start", "/*", true},
		{"just line comment", "--", true},
		{"DROP keyword", "1=1 DROP TABLE users", true},
		{"SELECT keyword", "1=1 UNION SELECT * FROM users", true},
		{"UNION keyword", "id IN (SELECT 1 UNION SELECT 2)", true},
		{"DELETE keyword", "DELETE FROM users", true},
		{"INSERT keyword", "INSERT INTO users VALUES (1)", true},
		{"UPDATE keyword", "UPDATE users SET name='x'", true},
		{"ALTER keyword", "ALTER TABLE users ADD x INT", true},
		{"CREATE keyword", "CREATE TABLE evil(id INT)", true},
		{"TRUNCATE keyword", "TRUNCATE users", true},
		{"GRANT keyword", "GRANT ALL ON users TO evil", true},
		{"REVOKE keyword", "REVOKE ALL ON users FROM evil", true},
		{"EXEC keyword", "EXEC sp_executesql", true},
		{"case insensitive drop", "drop table users", true},
		{"mixed case Union", "Union Select * from users", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSQLFragment(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSQLFragment(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
