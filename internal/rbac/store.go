package rbac

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

type Role struct {
	ID          int64
	Name        string
	Description *string
}

type Group struct {
	ID          int64
	AccountID   int64
	Name        string
	Description *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Members     []Member
}

type Member struct {
	UserID      int64
	Username    string
	DisplayName *string
}

type Permission struct {
	ID         int64
	AccountID  int64
	UserID     *int64
	RoleID     *int64
	RoleName   *string
	GroupID    *int64
	GroupName  *string
	BucketID   *int64
	BucketName *string
	Prefix     *string
	Actions    []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Template struct {
	ID          int64
	AccountID   int64
	Name        string
	Description *string
	Rules       []TemplateRule
	Assignments []Assignment
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TemplateRule struct {
	ID         int64
	TemplateID int64
	BucketID   *int64
	BucketName *string
	Prefix     *string
	Actions    []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Assignment struct {
	ID         int64
	AccountID  int64
	TemplateID int64
	UserID     *int64
	GroupID    *int64
}

func (s *Store) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, description FROM tenant_roles ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.Description); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) RoleByID(ctx context.Context, id int64) (*Role, error) {
	var r Role
	err := s.pool.QueryRow(ctx, `SELECT id, name, description FROM tenant_roles WHERE id = $1`, id).Scan(&r.ID, &r.Name, &r.Description)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListGroups(ctx context.Context, accountID int64) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, name, description, created_at, updated_at
		FROM tenant_user_groups WHERE account_id = $1 ORDER BY name`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		members, err := s.groupMembers(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Members = members
	}
	return out, nil
}

func (s *Store) GetGroup(ctx context.Context, accountID, id int64) (*Group, error) {
	g, err := scanGroup(s.pool.QueryRow(ctx, `
		SELECT id, account_id, name, description, created_at, updated_at
		FROM tenant_user_groups WHERE id = $1 AND account_id = $2`, id, accountID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.Members, err = s.groupMembers(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) InsertGroup(ctx context.Context, accountID int64, name string, desc *string) (*Group, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenant_user_groups (account_id, name, description) VALUES ($1,$2,$3)
		RETURNING id`, accountID, name, desc).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetGroup(ctx, accountID, id)
}

func (s *Store) UpdateGroup(ctx context.Context, accountID, id int64, name, desc *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tenant_user_groups SET
			name = COALESCE($3, name),
			description = COALESCE($4, description),
			updated_at = now()
		WHERE id = $1 AND account_id = $2`, id, accountID, name, desc)
	return err
}

func (s *Store) DeleteGroup(ctx context.Context, accountID, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tenant_user_groups WHERE id = $1 AND account_id = $2`, id, accountID)
	return err
}

func (s *Store) AddMembers(ctx context.Context, groupID int64, userIDs []int64) error {
	for _, uid := range userIDs {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO tenant_user_group_members (group_id, user_id) VALUES ($1,$2)
			ON CONFLICT (group_id, user_id) DO NOTHING`, groupID, uid); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RemoveMember(ctx context.Context, groupID, userID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tenant_user_group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	return err
}

func (s *Store) groupMembers(ctx context.Context, groupID int64) ([]Member, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.username, u.display_name
		FROM tenant_user_group_members m
		JOIN tenant_users u ON u.id = m.user_id
		WHERE m.group_id = $1
		ORDER BY u.username`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GroupIDs(ctx context.Context, accountID, userID int64) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.group_id FROM tenant_user_group_members m
		JOIN tenant_user_groups g ON g.id = m.group_id
		WHERE m.user_id = $1 AND g.account_id = $2`, userID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) ListPermissions(ctx context.Context, accountID int64) ([]Permission, error) {
	rows, err := s.pool.Query(ctx, permissionSelect+` WHERE p.account_id = $1 ORDER BY p.id`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()
	var out []Permission
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPermission(ctx context.Context, accountID, id int64) (*Permission, error) {
	p, err := scanPermission(s.pool.QueryRow(ctx, permissionSelect+` WHERE p.id = $1 AND p.account_id = $2`, id, accountID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) InsertPermission(ctx context.Context, p Permission) (*Permission, error) {
	raw, err := json.Marshal(p.Actions)
	if err != nil {
		return nil, err
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO tenant_permissions (account_id, user_id, role_id, group_id, bucket_id, prefix, actions)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		p.AccountID, p.UserID, p.RoleID, p.GroupID, p.BucketID, p.Prefix, raw).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetPermission(ctx, p.AccountID, id)
}

func (s *Store) UpdatePermission(ctx context.Context, accountID, id int64, bucketID *int64, prefix *string, actions []string) error {
	raw, err := json.Marshal(actions)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE tenant_permissions SET bucket_id = $3, prefix = $4, actions = $5, updated_at = now()
		WHERE id = $1 AND account_id = $2`, id, accountID, bucketID, prefix, raw)
	return err
}

func (s *Store) DeletePermission(ctx context.Context, accountID, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tenant_permissions WHERE id = $1 AND account_id = $2`, id, accountID)
	return err
}

const permissionSelect = `
	SELECT p.id, p.account_id, p.user_id, p.role_id, r.name, p.group_id, g.name,
		p.bucket_id, b.bucket_name, p.prefix, p.actions, p.created_at, p.updated_at
	FROM tenant_permissions p
	LEFT JOIN tenant_roles r ON r.id = p.role_id
	LEFT JOIN tenant_user_groups g ON g.id = p.group_id
	LEFT JOIN buckets b ON b.id = p.bucket_id`

func (s *Store) LoadRules(ctx context.Context, accountID, userID int64, role string) ([]Rule, error) {
	rows, err := s.pool.Query(ctx, permissionSelect+`
		WHERE p.account_id = $1 AND (
			p.user_id = $2
			OR r.name = $3
			OR p.group_id IN (
				SELECT m.group_id FROM tenant_user_group_members m
				JOIN tenant_user_groups gg ON gg.id = m.group_id
				WHERE m.user_id = $2 AND gg.account_id = $1
			)
		)`, accountID, userID, role)
	if err != nil {
		return nil, fmt.Errorf("load permissions: %w", err)
	}
	defer rows.Close()
	var rules []Rule
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, Rule{
			BucketName: p.BucketName, Prefix: p.Prefix, Actions: p.Actions,
			UserID: p.UserID, RoleName: p.RoleName, GroupID: p.GroupID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	trows, err := s.pool.Query(ctx, `
		SELECT b.bucket_name, tr.prefix, tr.actions
		FROM tenant_permission_template_rules tr
		JOIN tenant_permission_template_assignments a ON a.template_id = tr.template_id
		LEFT JOIN buckets b ON b.id = tr.bucket_id
		WHERE a.account_id = $1 AND (
			a.user_id = $2 OR a.group_id IN (
				SELECT m.group_id FROM tenant_user_group_members m
				JOIN tenant_user_groups gg ON gg.id = m.group_id
				WHERE m.user_id = $2 AND gg.account_id = $1
			)
		)`, accountID, userID)
	if err != nil {
		return nil, fmt.Errorf("load template rules: %w", err)
	}
	defer trows.Close()
	for trows.Next() {
		var bucket, prefix *string
		var raw []byte
		if err := trows.Scan(&bucket, &prefix, &raw); err != nil {
			return nil, err
		}
		uid := userID
		rules = append(rules, Rule{BucketName: bucket, Prefix: prefix, Actions: decodeActions(raw), UserID: &uid})
	}
	return rules, trows.Err()
}

func (s *Store) ListTemplates(ctx context.Context, accountID int64) ([]Template, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, name, description, created_at, updated_at
		FROM tenant_permission_templates WHERE account_id = $1 ORDER BY name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.AccountID, &t.Name, &t.Description, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.fillTemplate(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) GetTemplate(ctx context.Context, accountID, id int64) (*Template, error) {
	var t Template
	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, name, description, created_at, updated_at
		FROM tenant_permission_templates WHERE id = $1 AND account_id = $2`, id, accountID).
		Scan(&t.ID, &t.AccountID, &t.Name, &t.Description, &t.CreatedAt, &t.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.fillTemplate(ctx, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) InsertTemplate(ctx context.Context, accountID int64, name string, desc *string) (*Template, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenant_permission_templates (account_id, name, description) VALUES ($1,$2,$3)
		RETURNING id`, accountID, name, desc).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetTemplate(ctx, accountID, id)
}

func (s *Store) UpdateTemplate(ctx context.Context, accountID, id int64, name, desc *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tenant_permission_templates SET
			name = COALESCE($3, name), description = COALESCE($4, description), updated_at = now()
		WHERE id = $1 AND account_id = $2`, id, accountID, name, desc)
	return err
}

func (s *Store) DeleteTemplate(ctx context.Context, accountID, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tenant_permission_templates WHERE id = $1 AND account_id = $2`, id, accountID)
	return err
}

func (s *Store) InsertTemplateRule(ctx context.Context, templateID int64, bucketID *int64, prefix *string, actions []string) (*TemplateRule, error) {
	raw, err := json.Marshal(actions)
	if err != nil {
		return nil, err
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO tenant_permission_template_rules (template_id, bucket_id, prefix, actions)
		VALUES ($1,$2,$3,$4) RETURNING id`, templateID, bucketID, prefix, raw).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.getTemplateRule(ctx, templateID, id)
}

func (s *Store) UpdateTemplateRule(ctx context.Context, templateID, ruleID int64, bucketID *int64, prefix *string, actions []string) error {
	raw, err := json.Marshal(actions)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE tenant_permission_template_rules SET bucket_id = $3, prefix = $4, actions = $5, updated_at = now()
		WHERE id = $1 AND template_id = $2`, ruleID, templateID, bucketID, prefix, raw)
	return err
}

func (s *Store) DeleteTemplateRule(ctx context.Context, templateID, ruleID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tenant_permission_template_rules WHERE id = $1 AND template_id = $2`, ruleID, templateID)
	return err
}

func (s *Store) getTemplateRule(ctx context.Context, templateID, id int64) (*TemplateRule, error) {
	r, err := scanTemplateRule(s.pool.QueryRow(ctx, templateRuleSelect+` WHERE tr.id = $1 AND tr.template_id = $2`, id, templateID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) InsertAssignment(ctx context.Context, accountID, templateID int64, userID, groupID *int64) (*Assignment, error) {
	var a Assignment
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenant_permission_template_assignments (account_id, template_id, user_id, group_id)
		VALUES ($1,$2,$3,$4)
		RETURNING id, account_id, template_id, user_id, group_id`, accountID, templateID, userID, groupID).
		Scan(&a.ID, &a.AccountID, &a.TemplateID, &a.UserID, &a.GroupID)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) GetAssignment(ctx context.Context, accountID, templateID, id int64) (*Assignment, error) {
	var a Assignment
	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, template_id, user_id, group_id
		FROM tenant_permission_template_assignments
		WHERE id = $1 AND template_id = $2 AND account_id = $3`, id, templateID, accountID).
		Scan(&a.ID, &a.AccountID, &a.TemplateID, &a.UserID, &a.GroupID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) DeleteAssignment(ctx context.Context, accountID, templateID, id int64) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM tenant_permission_template_assignments
		WHERE id = $1 AND template_id = $2 AND account_id = $3`, id, templateID, accountID)
	return err
}

func (s *Store) fillTemplate(ctx context.Context, t *Template) error {
	rows, err := s.pool.Query(ctx, templateRuleSelect+` WHERE tr.template_id = $1 ORDER BY tr.id`, t.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	t.Rules = []TemplateRule{}
	for rows.Next() {
		r, err := scanTemplateRule(rows)
		if err != nil {
			return err
		}
		t.Rules = append(t.Rules, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	arows, err := s.pool.Query(ctx, `
		SELECT id, account_id, template_id, user_id, group_id
		FROM tenant_permission_template_assignments WHERE template_id = $1 ORDER BY id`, t.ID)
	if err != nil {
		return err
	}
	defer arows.Close()
	t.Assignments = []Assignment{}
	for arows.Next() {
		var a Assignment
		if err := arows.Scan(&a.ID, &a.AccountID, &a.TemplateID, &a.UserID, &a.GroupID); err != nil {
			return err
		}
		t.Assignments = append(t.Assignments, a)
	}
	return arows.Err()
}

const templateRuleSelect = `
	SELECT tr.id, tr.template_id, tr.bucket_id, b.bucket_name, tr.prefix, tr.actions, tr.created_at, tr.updated_at
	FROM tenant_permission_template_rules tr
	LEFT JOIN buckets b ON b.id = tr.bucket_id`

func scanTemplateRule(row userRow) (TemplateRule, error) {
	var r TemplateRule
	var raw []byte
	err := row.Scan(&r.ID, &r.TemplateID, &r.BucketID, &r.BucketName, &r.Prefix, &raw, &r.CreatedAt, &r.UpdatedAt)
	r.Actions = decodeActions(raw)
	return r, err
}

func scanGroup(row userRow) (Group, error) {
	var g Group
	err := row.Scan(&g.ID, &g.AccountID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt)
	return g, err
}

func scanPermission(row userRow) (Permission, error) {
	var p Permission
	var raw []byte
	err := row.Scan(
		&p.ID, &p.AccountID, &p.UserID, &p.RoleID, &p.RoleName, &p.GroupID, &p.GroupName,
		&p.BucketID, &p.BucketName, &p.Prefix, &raw, &p.CreatedAt, &p.UpdatedAt,
	)
	p.Actions = decodeActions(raw)
	return p, err
}

func decodeActions(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

type userRow interface {
	Scan(dest ...any) error
}
