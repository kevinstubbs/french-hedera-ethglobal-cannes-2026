package naryo

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/french-hedera-ethglobal-cannes2026/submission/internal/config"
)

// pipelineFilterPlan describes how to provision Naryo for one pipeline session.
type pipelineFilterPlan struct {
	UseALLFallback bool
	Reason         string
	Key            string // stable; used to detect config changes vs in-memory session state

	HederaTopicID string // Hedera HCS topic (shard.realm.num)
	EVMContract   string // token contract hex, lowercase 0x…
	// EVMFromFilterUnmet is true when the session asked for a specific transfer.from but Naryo
	// filter only scopes by contract + Transfer event (all transfers on that contract).
	EVMFromFilterUnmet bool
}

func resolvePipelineFilterPlan(cfg map[string]any) pipelineFilterPlan {
	allowALLFallback := config.NaryoAllowAllBroadcasterFallback()
	subs := extractSubscriptionsArray(cfg)

	for _, s := range subs {
		if subscriptionKind(s) != "hedera_hcs_topic" {
			continue
		}
		tid := strings.TrimSpace(stringField(s, "topicId"))
		if tid != "" && tid != "0.0.0" {
			return pipelineFilterPlan{Key: "H:" + tid, HederaTopicID: tid}
		}
	}
	defTopic := strings.TrimSpace(config.NaryoDefaultHCSTopicID())
	if defTopic != "" && defTopic != "0.0.0" {
		return pipelineFilterPlan{Key: "H:" + defTopic, HederaTopicID: defTopic}
	}
	for _, s := range subs {
		if subscriptionKind(s) != "erc20_transfer" {
			continue
		}
		ca := normalizeEVMAddress(stringField(s, "contractAddress"))
		if ca != "" {
			from := strings.TrimSpace(stringField(s, "fromAddress"))
			return pipelineFilterPlan{
				Key:                "E:" + ca,
				EVMContract:        ca,
				EVMFromFilterUnmet: from != "",
			}
		}
	}
	if allowALLFallback {
		return pipelineFilterPlan{
			UseALLFallback: true,
			Key:            "ALL",
			Reason:         "no hedera_hcs_topic or erc20_transfer in eventSubscriptions (and no NARYO_DEFAULT_HCS_TOPIC_ID)",
		}
	}
	return pipelineFilterPlan{
		UseALLFallback: false,
		Key:            "",
		Reason:         "no hedera_hcs_topic or erc20_transfer in eventSubscriptions (and no NARYO_DEFAULT_HCS_TOPIC_ID); set NARYO_ALLOW_ALL_BROADCASTER_FALLBACK=true to provision an ALL-target broadcaster",
	}
}

func extractSubscriptionsArray(cfg map[string]any) []map[string]any {
	if cfg == nil {
		return nil
	}
	raw, ok := cfg["eventSubscriptions"]
	if !ok || raw == nil {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	arr, ok := m["subscriptions"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		sm, ok := it.(map[string]any)
		if ok {
			out = append(out, sm)
		}
	}
	return out
}

func subscriptionKind(m map[string]any) string {
	return strings.TrimSpace(stringField(m, "kind"))
}

func stringField(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	v, ok := m[k]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return strings.TrimSpace(x.String())
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%.0f", x))
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

// DescribePipelineFilterPlanForSession summarizes how EnsurePipeline will provision Naryo from
// session config (for dashboards / observability). It does not call Naryo.
func DescribePipelineFilterPlanForSession(sessionID string, cfg map[string]any) map[string]any {
	p := resolvePipelineFilterPlan(cfg)
	subs := extractSubscriptionsArray(cfg)
	out := map[string]any{
		"key":            p.Key,
		"useALLFallback": p.UseALLFallback,
		"reason":         p.Reason,
		"naryoProvisioningSemantics": "The platform stores the full eventSubscriptions list on the session (OR intent for agents/UI). " +
			"EnsurePipeline sends at most one scoped Naryo filter per pipeline: first hedera_hcs_topic with topicId, " +
			"else NARYO_DEFAULT_HCS_TOPIC_ID, else first erc20_transfer, else ALL-target broadcaster when allowed. " +
			"hederaNetwork and caip2 in JSON are not passed to Naryo's Configuration API — Hedera mirror and EVM RPC come from Naryo's own application.yml / env.",
	}
	if p.HederaTopicID != "" {
		out["hederaTopicId"] = p.HederaTopicID
		out["expectedNaryoFilterName"] = "pf-" + sessionID + "-hcs"
		out["naryoFilterType"] = "TRANSACTION (IDENTITY_ID = HCS topic)"
		appendNaryoVersusSessionSubscriptions(out, subs, p)
	}
	if p.EVMContract != "" {
		out["evmContract"] = p.EVMContract
		out["evmFromFilterUnmet"] = p.EVMFromFilterUnmet
		if p.HederaTopicID == "" {
			out["expectedNaryoFilterName"] = "pf-" + sessionID + "-evm"
			out["naryoFilterType"] = "EVENT_CONTRACT (Transfer on token contract; fromAddress in config is not applied in Naryo)"
			appendNaryoVersusSessionSubscriptions(out, subs, p)
		}
	}
	if p.UseALLFallback {
		out["expectedNaryoFilterName"] = nil
		out["naryoFilterType"] = "none (ALL-target HTTP broadcaster only)"
		out["filterRowsNote"] = "ALL-target provisioning does not create a per-pipeline filter in Naryo (only an HTTP broadcaster). Scoped HCS/EVM plans create filters named pf-<sessionId>-hcs or -evm."
		var ids []string
		for _, s := range subs {
			if id := strings.TrimSpace(stringField(s, "id")); id != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			out["subscriptionsIntentOnly"] = ids
		}
	}
	return out
}

func appendNaryoVersusSessionSubscriptions(out map[string]any, subs []map[string]any, p pipelineFilterPlan) {
	var winningID string
	if p.HederaTopicID != "" {
		for _, s := range subs {
			if subscriptionKind(s) != "hedera_hcs_topic" {
				continue
			}
			if strings.TrimSpace(stringField(s, "topicId")) != p.HederaTopicID {
				continue
			}
			winningID = strings.TrimSpace(stringField(s, "id"))
			break
		}
		if winningID == "" {
			out["naryoTopicSource"] = "NARYO_DEFAULT_HCS_TOPIC_ID or first matching topic (not necessarily a subscription id below)"
		} else {
			out["naryoTopicSource"] = "eventSubscriptions"
			out["winningSubscriptionId"] = winningID
		}
		var notInNaryo []string
		for _, s := range subs {
			kind := subscriptionKind(s)
			sid := strings.TrimSpace(stringField(s, "id"))
			if sid == "" {
				sid = kind
			}
			switch kind {
			case "erc20_transfer":
				notInNaryo = append(notInNaryo, sid+": erc20_transfer — not provisioned while a Hedera topic filter is selected (single filter per pipeline).")
			case "hedera_hcs_topic":
				if p.HederaTopicID != "" && strings.TrimSpace(stringField(s, "topicId")) != p.HederaTopicID {
					notInNaryo = append(notInNaryo, sid+": hedera_hcs_topic — different topicId; only one Hedera filter is provisioned.")
				}
			}
		}
		if len(notInNaryo) > 0 {
			out["subscriptionsNotProvisionedInNaryo"] = notInNaryo
		}
		return
	}
	if p.EVMContract != "" {
		for _, s := range subs {
			if subscriptionKind(s) != "erc20_transfer" {
				continue
			}
			if normalizeEVMAddress(stringField(s, "contractAddress")) != p.EVMContract {
				continue
			}
			out["winningSubscriptionId"] = strings.TrimSpace(stringField(s, "id"))
			break
		}
		var notInNaryo []string
		for _, s := range subs {
			kind := subscriptionKind(s)
			sid := strings.TrimSpace(stringField(s, "id"))
			if sid == "" {
				sid = kind
			}
			if kind == "hedera_hcs_topic" {
				notInNaryo = append(notInNaryo, sid+": hedera_hcs_topic — not provisioned (no valid topic before EVM in resolution order, or topic empty).")
			}
			if kind == "erc20_transfer" && normalizeEVMAddress(stringField(s, "contractAddress")) != p.EVMContract {
				notInNaryo = append(notInNaryo, sid+": erc20_transfer — different contract; only first matching erc20_transfer is provisioned.")
			}
		}
		if len(notInNaryo) > 0 {
			out["subscriptionsNotProvisionedInNaryo"] = notInNaryo
		}
	}
}

func normalizeEVMAddress(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "0x") {
		s = "0x" + s
	}
	return s
}
