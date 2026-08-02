package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/FUjr/tvpn/internal/auth"
	"github.com/FUjr/tvpn/internal/httpapi"
	"github.com/FUjr/tvpn/internal/ldapauth"
	"github.com/FUjr/tvpn/internal/policy"
	proxyservice "github.com/FUjr/tvpn/internal/proxy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HTTP struct {
	db        *pgxpool.Pool
	authStore *auth.Store
	ldap      *ldapauth.Service
	upstreams *proxyservice.UpstreamStore
}

func NewHTTP(db *pgxpool.Pool, authStore *auth.Store, ldap *ldapauth.Service, upstreams *proxyservice.UpstreamStore) *HTTP {
	return &HTTP{db: db, authStore: authStore, ldap: ldap, upstreams: upstreams}
}

func (h *HTTP) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/users", h.listUsers)
	r.Post("/users", h.createUser)
	r.Patch("/users/{id}", h.updateUser)
	r.Post("/users/{id}/password", h.setPassword)
	r.Put("/users/{id}/policies", h.setUserPolicies)
	r.Get("/policies", h.listPolicies)
	r.Post("/policies", h.createPolicy)
	r.Put("/policies/{id}", h.updatePolicy)
	r.Delete("/policies/{id}", h.deletePolicy)
	r.Get("/ldap", h.getLDAP)
	r.Put("/ldap", h.updateLDAP)
	r.Post("/ldap/test", h.testLDAP)
	r.Get("/ldap/groups", h.listGroups)
	r.Put("/ldap/groups/{id}/policies", h.setGroupPolicies)
	r.Get("/upstream-proxies", h.listUpstreamProxies)
	r.Post("/upstream-proxies", h.createUpstreamProxy)
	r.Put("/upstream-proxies/{id}", h.updateUpstreamProxy)
	r.Delete("/upstream-proxies/{id}", h.deleteUpstreamProxy)
	r.Put("/upstream-proxies/{id}/users", h.setUpstreamProxyUsers)
	r.Put("/upstream-proxies/{id}/groups", h.setUpstreamProxyGroups)
	r.Get("/audit", h.listAudit)
	return r
}

func (h *HTTP) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `SELECT u.id,u.username,u.display_name,u.email,u.auth_source,u.is_admin,u.disabled_at,u.last_login_at,
		COALESCE(array_agg(up.policy_id) FILTER (WHERE up.policy_id IS NOT NULL),'{}') FROM users u LEFT JOIN user_policies up ON up.user_id=u.id
		GROUP BY u.id ORDER BY u.username`)
	if err != nil {
		internal(w)
		return
	}
	defer rows.Close()
	type item struct {
		ID          uuid.UUID   `json:"id"`
		Username    string      `json:"username"`
		DisplayName string      `json:"display_name"`
		Email       string      `json:"email"`
		AuthSource  string      `json:"auth_source"`
		IsAdmin     bool        `json:"is_admin"`
		DisabledAt  *time.Time  `json:"disabled_at"`
		LastLoginAt *time.Time  `json:"last_login_at"`
		PolicyIDs   []uuid.UUID `json:"policy_ids"`
	}
	values := []item{}
	for rows.Next() {
		var value item
		if rows.Scan(&value.ID, &value.Username, &value.DisplayName, &value.Email, &value.AuthSource, &value.IsAdmin, &value.DisabledAt, &value.LastLoginAt, &value.PolicyIDs) != nil {
			internal(w)
			return
		}
		values = append(values, value)
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (h *HTTP) createUser(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		IsAdmin     bool   `json:"is_admin"`
	}
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	user, err := h.authStore.CreateLocalUser(r.Context(), input.Username, input.DisplayName, input.Email, input.Password, input.IsAdmin)
	if err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_user", err.Error())
		return
	}
	h.audit(r, "user.create", "success", user.ID.String(), nil)
	httpapi.WriteJSON(w, http.StatusCreated, user)
}

func (h *HTTP) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var input struct {
		DisplayName *string `json:"display_name"`
		Email       *string `json:"email"`
		IsAdmin     *bool   `json:"is_admin"`
		Disabled    *bool   `json:"disabled"`
	}
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	if (input.IsAdmin != nil && !*input.IsAdmin) || (input.Disabled != nil && *input.Disabled) {
		var active bool
		var count int
		if h.db.QueryRow(r.Context(), `SELECT is_admin AND disabled_at IS NULL FROM users WHERE id=$1`, id).Scan(&active) == nil && active {
			_ = h.db.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE is_admin=true AND disabled_at IS NULL`).Scan(&count)
			if count <= 1 {
				httpapi.Problem(w, http.StatusConflict, "last_admin", "不能禁用或降级最后一个管理员")
				return
			}
		}
	}
	_, err := h.db.Exec(r.Context(), `UPDATE users SET display_name=COALESCE($2,display_name),email=COALESCE($3,email),is_admin=COALESCE($4,is_admin),disabled_at=CASE WHEN $5::boolean IS NULL THEN disabled_at WHEN $5 THEN now() ELSE NULL END,updated_at=now() WHERE id=$1`, id, input.DisplayName, input.Email, input.IsAdmin, input.Disabled)
	if err != nil {
		internal(w)
		return
	}
	h.audit(r, "user.update", "success", id.String(), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) setPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_password", err.Error())
		return
	}
	tag, err := h.db.Exec(r.Context(), `UPDATE users SET password_hash=$2,updated_at=now() WHERE id=$1 AND auth_source='local'`, id, hash)
	if err != nil || tag.RowsAffected() != 1 {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_user", "只能重置本地用户密码")
		return
	}
	_, _ = h.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, id)
	h.audit(r, "user.password", "success", id.String(), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) setUserPolicies(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var input assignment
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	if err := replaceAssignments(r.Context(), h.db, "user_policies", "user_id", id, input.PolicyIDs); err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_policy", err.Error())
		return
	}
	h.audit(r, "user.policies", "success", id.String(), map[string]any{"count": len(input.PolicyIDs)})
	w.WriteHeader(http.StatusNoContent)
}

type assignment struct {
	PolicyIDs []uuid.UUID `json:"policy_ids"`
}
type policyInput struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Mode        policy.Mode `json:"mode"`
	Enabled     bool        `json:"enabled"`
	Rules       []struct {
		Kind  policy.RuleKind `json:"kind"`
		Value string          `json:"value"`
	} `json:"rules"`
}

func (h *HTTP) listPolicies(w http.ResponseWriter, r *http.Request) {
	values, err := h.policies(r.Context())
	if err != nil {
		internal(w)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"items": values})
}
func (h *HTTP) policies(ctx context.Context) ([]policy.Policy, error) {
	rows, err := h.db.Query(ctx, `SELECT id,name,description,mode,enabled FROM policies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []policy.Policy{}
	for rows.Next() {
		value := policy.Policy{Rules: []policy.Rule{}}
		if err := rows.Scan(&value.ID, &value.Name, &value.Description, &value.Mode, &value.Enabled); err != nil {
			return nil, err
		}
		ruleRows, err := h.db.Query(ctx, `SELECT id,kind,value FROM policy_rules WHERE policy_id=$1 ORDER BY kind,value`, value.ID)
		if err != nil {
			return nil, err
		}
		for ruleRows.Next() {
			var rule policy.Rule
			if err := ruleRows.Scan(&rule.ID, &rule.Kind, &rule.Value); err != nil {
				ruleRows.Close()
				return nil, err
			}
			value.Rules = append(value.Rules, rule)
		}
		ruleRows.Close()
		values = append(values, value)
	}
	return values, rows.Err()
}
func (h *HTTP) createPolicy(w http.ResponseWriter, r *http.Request) {
	var input policyInput
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	id := uuid.New()
	if err := h.savePolicy(r.Context(), id, input, true); err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_policy", err.Error())
		return
	}
	h.audit(r, "policy.create", "success", id.String(), nil)
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"id": id})
}
func (h *HTTP) updatePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var input policyInput
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	if err := h.savePolicy(r.Context(), id, input, false); err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_policy", err.Error())
		return
	}
	h.audit(r, "policy.update", "success", id.String(), nil)
	w.WriteHeader(http.StatusNoContent)
}
func (h *HTTP) savePolicy(ctx context.Context, id uuid.UUID, input policyInput, create bool) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return errors.New("name is required")
	}
	if input.Mode != policy.DenyAll && input.Mode != policy.DenyIntranet && input.Mode != policy.Whitelist && input.Mode != policy.Blacklist {
		return errors.New("invalid mode")
	}
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if create {
		_, err = tx.Exec(ctx, `INSERT INTO policies(id,name,description,mode,enabled)VALUES($1,$2,$3,$4,$5)`, id, input.Name, input.Description, input.Mode, input.Enabled)
	} else {
		tag, e := tx.Exec(ctx, `UPDATE policies SET name=$2,description=$3,mode=$4,enabled=$5,updated_at=now() WHERE id=$1`, id, input.Name, input.Description, input.Mode, input.Enabled)
		err = e
		if err == nil && tag.RowsAffected() != 1 {
			return errors.New("policy not found")
		}
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM policy_rules WHERE policy_id=$1`, id); err != nil {
		return err
	}
	for _, item := range input.Rules {
		value, e := policy.ValidateRule(item.Kind, item.Value)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO policy_rules(id,policy_id,kind,value)VALUES($1,$2,$3,$4)`, uuid.New(), id, item.Kind, value); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (h *HTTP) deletePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	_, err := h.db.Exec(r.Context(), `DELETE FROM policies WHERE id=$1`, id)
	if err != nil {
		internal(w)
		return
	}
	h.audit(r, "policy.delete", "success", id.String(), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) getLDAP(w http.ResponseWriter, r *http.Request) {
	value, err := h.ldap.Settings(r.Context())
	if err != nil {
		internal(w)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"settings": value, "bind_password_configured": h.ldap.BindPasswordConfigured()})
}
func (h *HTTP) updateLDAP(w http.ResponseWriter, r *http.Request) {
	var value ldapauth.Settings
	if !httpapi.DecodeJSON(w, r, &value) {
		return
	}
	if err := h.ldap.UpdateSettings(r.Context(), value); err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_ldap", err.Error())
		return
	}
	h.audit(r, "ldap.update", "success", "ldap", nil)
	w.WriteHeader(http.StatusNoContent)
}
func (h *HTTP) testLDAP(w http.ResponseWriter, r *http.Request) {
	if err := h.ldap.Test(r.Context()); err != nil {
		h.audit(r, "ldap.test", "failure", "ldap", map[string]any{"error": "connection failed"})
		httpapi.Problem(w, http.StatusBadGateway, "ldap_unavailable", "LDAP 连接或服务绑定失败")
		return
	}
	h.audit(r, "ldap.test", "success", "ldap", nil)
	w.WriteHeader(http.StatusNoContent)
}
func (h *HTTP) listGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `SELECT g.id,g.external_dn,g.name,g.last_seen_at,COALESCE(array_agg(gp.policy_id)FILTER(WHERE gp.policy_id IS NOT NULL),'{}')FROM ldap_groups g LEFT JOIN ldap_group_policies gp ON gp.group_id=g.id GROUP BY g.id ORDER BY g.name`)
	if err != nil {
		internal(w)
		return
	}
	defer rows.Close()
	type item struct {
		ID         uuid.UUID   `json:"id"`
		ExternalDN string      `json:"external_dn"`
		Name       string      `json:"name"`
		LastSeenAt time.Time   `json:"last_seen_at"`
		PolicyIDs  []uuid.UUID `json:"policy_ids"`
	}
	values := []item{}
	for rows.Next() {
		var value item
		if rows.Scan(&value.ID, &value.ExternalDN, &value.Name, &value.LastSeenAt, &value.PolicyIDs) != nil {
			internal(w)
			return
		}
		values = append(values, value)
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"items": values})
}
func (h *HTTP) setGroupPolicies(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var input assignment
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	if err := replaceAssignments(r.Context(), h.db, "ldap_group_policies", "group_id", id, input.PolicyIDs); err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_policy", err.Error())
		return
	}
	h.audit(r, "group.policies", "success", id.String(), map[string]any{"count": len(input.PolicyIDs)})
	w.WriteHeader(http.StatusNoContent)
}

type resourceAssignment struct {
	IDs []uuid.UUID `json:"ids"`
}

func (h *HTTP) listUpstreamProxies(w http.ResponseWriter, r *http.Request) {
	values, err := h.upstreams.List(r.Context())
	if err != nil {
		internal(w)
		return
	}
	type item struct {
		proxyservice.Upstream
		UserIDs  []uuid.UUID `json:"user_ids"`
		GroupIDs []uuid.UUID `json:"group_ids"`
	}
	items := make([]item, 0, len(values))
	for _, value := range values {
		entry := item{Upstream: value, UserIDs: []uuid.UUID{}, GroupIDs: []uuid.UUID{}}
		userRows, queryErr := h.db.Query(r.Context(), `SELECT user_id FROM user_upstream_proxies WHERE upstream_proxy_id=$1 ORDER BY user_id`, value.ID)
		if queryErr != nil {
			internal(w)
			return
		}
		for userRows.Next() {
			var id uuid.UUID
			if userRows.Scan(&id) != nil {
				userRows.Close()
				internal(w)
				return
			}
			entry.UserIDs = append(entry.UserIDs, id)
		}
		userRows.Close()
		groupRows, queryErr := h.db.Query(r.Context(), `SELECT group_id FROM ldap_group_upstream_proxies WHERE upstream_proxy_id=$1 ORDER BY group_id`, value.ID)
		if queryErr != nil {
			internal(w)
			return
		}
		for groupRows.Next() {
			var id uuid.UUID
			if groupRows.Scan(&id) != nil {
				groupRows.Close()
				internal(w)
				return
			}
			entry.GroupIDs = append(entry.GroupIDs, id)
		}
		groupRows.Close()
		items = append(items, entry)
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *HTTP) createUpstreamProxy(w http.ResponseWriter, r *http.Request) {
	var input proxyservice.UpstreamInput
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	id, err := h.upstreams.Create(r.Context(), input)
	if err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_upstream_proxy", err.Error())
		return
	}
	h.audit(r, "upstream_proxy.create", "success", id.String(), map[string]any{"type": input.Type})
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *HTTP) updateUpstreamProxy(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var input proxyservice.UpstreamInput
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	if err := h.upstreams.Update(r.Context(), id, input); err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_upstream_proxy", err.Error())
		return
	}
	h.audit(r, "upstream_proxy.update", "success", id.String(), map[string]any{"type": input.Type})
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) deleteUpstreamProxy(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if err := h.upstreams.Delete(r.Context(), id); err != nil {
		httpapi.Problem(w, http.StatusNotFound, "upstream_proxy_not_found", "上游代理不存在")
		return
	}
	h.audit(r, "upstream_proxy.delete", "success", id.String(), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) setUpstreamProxyUsers(w http.ResponseWriter, r *http.Request) {
	h.setUpstreamAssignments(w, r, "user_upstream_proxies", "user_id", "upstream_proxy.users")
}

func (h *HTTP) setUpstreamProxyGroups(w http.ResponseWriter, r *http.Request) {
	h.setUpstreamAssignments(w, r, "ldap_group_upstream_proxies", "group_id", "upstream_proxy.groups")
}

func (h *HTTP) setUpstreamAssignments(w http.ResponseWriter, r *http.Request, table, column, eventType string) {
	id, ok := parseID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var input resourceAssignment
	if !httpapi.DecodeJSON(w, r, &input) {
		return
	}
	if err := replaceResourceAssignments(r.Context(), h.db, table, column, id, input.IDs); err != nil {
		httpapi.Problem(w, http.StatusUnprocessableEntity, "invalid_assignment", err.Error())
		return
	}
	h.audit(r, eventType, "success", id.String(), map[string]any{"count": len(input.IDs)})
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) listAudit(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `SELECT id,actor_user_id,event_type,outcome,target,detail,created_at FROM audit_events ORDER BY id DESC LIMIT 200`)
	if err != nil {
		internal(w)
		return
	}
	defer rows.Close()
	type item struct {
		ID          int64           `json:"id"`
		ActorUserID *uuid.UUID      `json:"actor_user_id"`
		EventType   string          `json:"event_type"`
		Outcome     string          `json:"outcome"`
		Target      string          `json:"target"`
		Detail      json.RawMessage `json:"detail"`
		CreatedAt   time.Time       `json:"created_at"`
	}
	values := []item{}
	for rows.Next() {
		var value item
		if rows.Scan(&value.ID, &value.ActorUserID, &value.EventType, &value.Outcome, &value.Target, &value.Detail, &value.CreatedAt) != nil {
			internal(w)
			return
		}
		values = append(values, value)
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"items": values})
}
func (h *HTTP) audit(r *http.Request, eventType, outcome, target string, detail map[string]any) {
	session, _ := auth.SessionFromContext(r.Context())
	if detail == nil {
		detail = map[string]any{}
	}
	raw, _ := json.Marshal(detail)
	_, _ = h.db.Exec(r.Context(), `INSERT INTO audit_events(actor_user_id,event_type,outcome,target,detail)VALUES($1,$2,$3,$4,$5)`, session.User.ID, eventType, outcome, target, raw)
}

func replaceAssignments(ctx context.Context, db *pgxpool.Pool, table, column string, id uuid.UUID, policies []uuid.UUID) error {
	if table != "user_policies" && table != "ldap_group_policies" {
		return errors.New("invalid assignment table")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "DELETE FROM "+table+" WHERE "+column+"=$1", id); err != nil {
		return err
	}
	for _, policyID := range policies {
		if _, err = tx.Exec(ctx, "INSERT INTO "+table+" ("+column+",policy_id) VALUES ($1,$2)", id, policyID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func replaceResourceAssignments(ctx context.Context, db *pgxpool.Pool, table, column string, proxyID uuid.UUID, ids []uuid.UUID) error {
	if (table != "user_upstream_proxies" || column != "user_id") && (table != "ldap_group_upstream_proxies" || column != "group_id") {
		return errors.New("invalid assignment table")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "DELETE FROM "+table+" WHERE upstream_proxy_id=$1", proxyID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.Exec(ctx, "INSERT INTO "+table+" ("+column+",upstream_proxy_id) VALUES ($1,$2)", id, proxyID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func parseID(w http.ResponseWriter, value string) (uuid.UUID, bool) {
	id, err := uuid.Parse(value)
	if err != nil {
		httpapi.Problem(w, http.StatusBadRequest, "invalid_id", "ID 格式无效")
		return uuid.Nil, false
	}
	return id, true
}
func internal(w http.ResponseWriter) {
	httpapi.Problem(w, http.StatusInternalServerError, "internal_error", "服务器内部错误")
}
