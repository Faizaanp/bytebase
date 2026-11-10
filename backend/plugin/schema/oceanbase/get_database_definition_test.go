package oceanbase

import (
	"testing"

	metadatapb "github.com/bytebase/omni/metadata"
	"github.com/stretchr/testify/require"

	storepb "github.com/bytebase/bytebase/backend/generated-go/store"
	"github.com/bytebase/bytebase/backend/plugin/schema"
)

// TestGetDatabaseDefinition pins the DDL this package writes for each kind of object,
// and that OceanBase reaches it through the registry. The input is hand-written
// metadata rather than a schema text, so no generator branch is capped by what the
// parser happens to extract; parsing is covered in get_database_metadata_test.go.
func TestGetDatabaseDefinition(t *testing.T) {
	table := &metadatapb.TableMetadata{
		Name: "t",
		Columns: []*metadatapb.ColumnMetadata{
			{Name: "id", Type: "int"},
			{Name: "name", Type: "varchar(64)", Nullable: true, Default: "'anon'"},
		},
		Indexes: []*metadatapb.IndexMetadata{
			{Name: "PRIMARY", Expressions: []string{"id"}, Primary: true, Unique: true},
		},
	}
	const tableDDL = "--\n" +
		"-- Table structure for `t`\n" +
		"--\n" +
		"CREATE TABLE `t` (\n" +
		"  `id` int NOT NULL,\n" +
		"  `name` varchar(64) DEFAULT 'anon',\n" +
		"  PRIMARY KEY (`id`)\n" +
		");\n\n"

	for _, tc := range []struct {
		name string
		want string
		get  func() (string, error)
	}{
		{
			name: "table",
			want: tableDDL,
			get: func() (string, error) {
				return schema.GetTableDefinition(storepb.Engine_OCEANBASE, "", table, nil)
			},
		},
		{
			name: "view",
			want: "--\n-- View structure for `v`\n--\nCREATE OR REPLACE VIEW `v` AS SELECT `id` FROM `t`;\n\n",
			get: func() (string, error) {
				return schema.GetViewDefinition(storepb.Engine_OCEANBASE, "", &metadatapb.ViewMetadata{Name: "v", Definition: "SELECT `id` FROM `t`"})
			},
		},
		{
			name: "function",
			want: "--\n-- Function structure for `f`\n--\nCREATE FUNCTION `f`() RETURNS int RETURN 1;;\nDELIMITER ;\n\n",
			get: func() (string, error) {
				return schema.GetFunctionDefinition(storepb.Engine_OCEANBASE, "", &metadatapb.FunctionMetadata{Name: "f", Definition: "CREATE FUNCTION `f`() RETURNS int RETURN 1"})
			},
		},
		{
			name: "procedure",
			want: "--\n-- Procedure structure for `p`\n--\nDELIMITER ;;\nCREATE PROCEDURE `p`() BEGIN END;;\nDELIMITER ;\n\n",
			get: func() (string, error) {
				return schema.GetProcedureDefinition(storepb.Engine_OCEANBASE, "", &metadatapb.ProcedureMetadata{Name: "p", Definition: "CREATE PROCEDURE `p`() BEGIN END"})
			},
		},
		{
			name: "database",
			want: tableDDL,
			get: func() (string, error) {
				return schema.GetDatabaseDefinition(storepb.Engine_OCEANBASE, schema.GetDefinitionContext{}, &metadatapb.DatabaseSchemaMetadata{
					Name:    "db",
					Schemas: []*metadatapb.SchemaMetadata{{Name: "", Tables: []*metadatapb.TableMetadata{table}}},
				})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition, err := tc.get()
			require.NoError(t, err)
			require.Equal(t, tc.want, definition)
		})
	}
}

func TestGetTableDefinitionRegexCheckConstraintRoundtrip(t *testing.T) {
	ddls := []string{
		"CREATE TABLE `t` (\n  `id` int NOT NULL,\n  `channel_code` varchar(10) NOT NULL,\n  CONSTRAINT `chk_channel` CHECK (REGEXP_LIKE(channel_code, '^[A-Z]{2}$'))\n);",
		"CREATE TABLE `t` (\n  `id` int NOT NULL,\n  `channel_code` varchar(10) NOT NULL,\n  CONSTRAINT `chk_channel` CHECK ((channel_code REGEXP '^[A-Z]{2}$'))\n);",
	}
	for _, ddl := range ddls {
		meta, err := schema.GetDatabaseMetadata(storepb.Engine_OCEANBASE, ddl)
		require.NoError(t, err)
		tbl := meta.GetSchemas()[0].GetTables()[0]
		got, err := schema.GetTableDefinition(storepb.Engine_OCEANBASE, "", tbl, nil)
		require.NoError(t, err)
		t.Logf("Generated definition:\n%s", got)
		// Try to reserialize / parse the generated table definition
		_, err = schema.GetDatabaseMetadata(storepb.Engine_OCEANBASE, got)
		require.NoError(t, err, "failed to reparse generated table definition: %s", got)
	}
}

func TestFormatCheckExpression(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"(`age` >= 18)", "(`age` >= 18)"},
		{"((`type` = 'A') and (`amount` is not null))", "((`type` = 'A') and (`amount` is not null))"},
		{"regexp_like(`channel_code`,'^[A-Z]{2}$')", "(regexp_like(`channel_code`,'^[A-Z]{2}$'))"},
		{"json_valid(`metadata`)", "(json_valid(`metadata`))"},
		{"(`a` > 0) and (`b` > 0)", "((`a` > 0) and (`b` > 0))"},
		{"(`val` = ')')", "(`val` = ')')"},
		{"(`val` = '\\')')", "(`val` = '\\')')"},
	} {
		require.Equal(t, tc.want, formatCheckExpression(tc.input), "input: %s", tc.input)
	}
}
