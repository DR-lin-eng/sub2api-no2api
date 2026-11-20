package sitemigration

import (
	"bytes"
	"encoding/json"
)

// These dependencies are not declared with a PostgreSQL FK. The parent rows
// must have been inserted by this same merge before their IDs can be used.
var jsonParents = map[string][]string{
	"accounts":                            {"tls_fingerprint_profiles"},
	"groups":                              {"accounts"},
	"announcements":                       {"groups"},
	"act_campaigns":                       {"groups"},
	"act_checkin_records":                 {"groups"},
	"act_participation_records":           {"groups"},
	"channel_account_stats_pricing_rules": {"groups", "accounts"},
	"ops_alert_rules":                     {"groups"},
	"ops_alert_events":                    {"groups"},
	"ops_error_logs":                      {"accounts"},
	"account_quality_runs":                {"accounts", "proxies", "tls_fingerprint_profiles"},
}

type lookupRef func(table, key, source string) (string, bool, error)
type referenceConflict struct{ table, field string }

func (e *referenceConflict) Error() string {
	return "关联记录未导入或存在冲突：" + e.table + "（" + e.field + "）"
}

// Policy, physical resources and unfinished financial work already have a
// destination owner. Installing these into another running system can redirect
// its traffic, execute jobs twice, or run a charge against the wrong user.
func mergeBoundary(name string, r row) string {
	switch name {
	case "custom_model_configs", "custom_model_request_templates", "error_passthrough_rules":
		return "模型模板及错误处理规则属于接收站的全局网关策略"
	case "payment_provider_instances":
		return "支付商户路由属于接收站，合并时保留现有收款配置"
	case "ipv6_egress_pools", "account_egress_bindings":
		return "源站的网络设备与节点绑定无法在接收站验证"
	case "usage_billing_dead_letters":
		return "待人工处理的结算包含源站的入账指令和幂等指纹，须先在源站对账"
	case "users":
		if amount := r["frozen_balance"]; amount != nil && !decimalZero(amount) {
			return "用户仍有冻结金额，请在源站完成结算再同步"
		}
	case "video_balance_holds":
		if r.str("status") != "settled" && r.str("status") != "released" {
			return "视频冻结尚未完成结算，保留源站任务归属"
		}
	case "batch_image_jobs":
		switch r.str("status") {
		case "completed", "cancelled", "failed", "output_deleted":
		default:
			return "批量任务尚未结束，保留源站任务归属"
		}
		if (r.str("provider_input_ref") != "" || r.str("gcs_input_uri") != "") && idText(r, "input_deleted_at") == "null" ||
			(r.str("provider_output_ref") != "" || r.str("gcs_output_uri") != "") && idText(r, "output_deleted_at") == "null" {
			return "批量任务仍拥有上游文件，接收站清理器可能删除源站对象"
		}
	case "payment_orders":
		switch r.str("status") {
		case "COMPLETED", "PARTIALLY_REFUNDED", "REFUNDED":
		default:
			return "订单仍可能触发核验、入账或退款，合并时保留源站处理权"
		}
	case "accounts":
		if r.str("egress_mode") == "ipv6_pool" {
			return "上游账号仍使用源站 IPv6 出口绑定"
		}
	}
	return ""
}
func decimalZero(v []byte) bool {
	for _, b := range v {
		if b != '0' && b != '.' && b != '-' {
			return false
		}
	}
	return true
}

// Field names here have meaning only within the specified table/column.
// In particular account.credentials contains upstream-provider account IDs,
// which must never be interpreted as local account IDs.
type nestedRef struct{ field, parent string }

var idFields = map[string]map[string][]nestedRef{
	"api_keys":                            {"group_bindings": {{"group_id", "groups"}}},
	"groups":                              {"model_routing": {{"*", "accounts"}}},
	"accounts":                            {"extra": {{"tls_fingerprint_profile_id", "tls_fingerprint_profiles"}}},
	"channel_account_stats_pricing_rules": {"group_ids": {{"", "groups"}}, "account_ids": {{"", "accounts"}}},
	"announcements":                       {"targeting": {{"group_ids", "groups"}}},
	"act_campaigns":                       {"config_json": {{"reward_group_id", "groups"}, {"required_group_ids", "groups"}}},
	"act_checkin_records":                 {"reward_payload_json": {{"reward_group_id", "groups"}}},
	"act_participation_records":           {"reward_payload_json": {{"reward_group_id", "groups"}}},
	"payment_orders":                      {"provider_snapshot": {{"provider_instance_id", "payment_provider_instances"}}},
	"ops_alert_rules":                     {"filters": {{"group_id", "groups"}}},
	"ops_alert_events":                    {"dimensions": {{"group_id", "groups"}}},
	"ops_error_logs":                      {"upstream_errors": {{"account_id", "accounts"}}},
	"account_quality_runs":                {"probe_details": {{"account_id", "accounts"}, {"proxy_id", "proxies"}, {"tls_fingerprint_profile_id", "tls_fingerprint_profiles"}}},
}

func rewriteEmbedded(name string, r row, lookup lookupRef) error {
	for col, refs := range idFields[name] {
		raw := r[col]
		if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
			continue
		}
		textJSON := name == "act_campaigns" && col == "config_json"
		if textJSON {
			raw = []byte(r.str(col))
			if len(raw) == 0 {
				continue
			}
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var data any
		if err := decoder.Decode(&data); err != nil {
			return &referenceConflict{name, col}
		}
		for _, ref := range refs {
			var err error
			if ref.field == "" {
				data, err = mapReference(data, ref, lookup, col)
			} else {
				err = rewriteObject(data, ref, lookup, col, 0)
			}
			if err != nil {
				return err
			}
		}
		out, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if textJSON {
			r.set(col, string(out))
		} else {
			r[col] = out
		}
	}
	return nil
}
func mapReference(value any, ref nestedRef, lookup lookupRef, col string) (any, error) {
	var id string
	switch v := value.(type) {
	case nil:
		return nil, nil
	case json.Number:
		id = v.String()
	case string:
		id = v
	case []any:
		for i, item := range v {
			switch item.(type) {
			case json.Number, string:
			default:
				return nil, &referenceConflict{ref.parent, col}
			}
			dst, err := mapReference(item, ref, lookup, col)
			if err != nil {
				return nil, err
			}
			v[i] = dst
		}
		return v, nil
	default:
		return nil, &referenceConflict{ref.parent, col}
	}
	// 0 and empty are ordinary unset sentinels in stored optional settings.
	if id == "0" || id == "" {
		return value, nil
	}
	mapped, ok, err := lookup(ref.parent, "id", id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &referenceConflict{ref.parent, col}
	}
	if _, s := value.(string); s {
		return mapped, nil
	}
	return json.Number(mapped), nil
}
func rewriteObject(obj any, ref nestedRef, lookup lookupRef, col string, depth int) error {
	if depth > 64 {
		return &referenceConflict{ref.parent, col}
	}
	switch v := obj.(type) {
	case map[string]any:
		for k, child := range v {
			if k == ref.field || ref.field == "*" {
				dest, err := mapReference(child, ref, lookup, col)
				if err != nil {
					return err
				}
				v[k] = dest
			} else if err := rewriteObject(child, ref, lookup, col, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := rewriteObject(child, ref, lookup, col, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
