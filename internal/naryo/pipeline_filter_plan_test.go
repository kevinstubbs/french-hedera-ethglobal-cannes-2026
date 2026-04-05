package naryo

import (
	"testing"
)

func TestResolvePipelineFilterPlanHederaSubscriptionWithAllowAllFallbackDefault(t *testing.T) {
	// Default NARYO_ALLOW_ALL_BROADCASTER_FALLBACK is true; Hedera subscription must still win over ALL.
	cfg := map[string]any{
		"eventSubscriptions": map[string]any{
			"subscriptions": []any{
				map[string]any{"kind": "hedera_hcs_topic", "topicId": "0.0.123"},
			},
		},
	}
	p := resolvePipelineFilterPlan(cfg)
	if p.UseALLFallback {
		t.Fatalf("unexpected fallback: %q", p.Reason)
	}
	if p.Key != "H:0.0.123" || p.HederaTopicID != "0.0.123" {
		t.Fatalf("got %+v", p)
	}
}

func TestResolvePipelineFilterPlanEVMSubscriptionWithAllowAllFallbackDefault(t *testing.T) {
	cfg := map[string]any{
		"eventSubscriptions": map[string]any{
			"subscriptions": []any{
				map[string]any{
					"kind":            "erc20_transfer",
					"contractAddress": "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
				},
			},
		},
	}
	p := resolvePipelineFilterPlan(cfg)
	if p.UseALLFallback || p.EVMContract != "0x036cbd53842c5426634e7929541ec2318f3dcf7e" {
		t.Fatalf("got %+v", p)
	}
}

func TestResolvePipelineFilterPlanEVMSubscription(t *testing.T) {
	t.Setenv("NARYO_ALLOW_ALL_BROADCASTER_FALLBACK", "false")
	cfg := map[string]any{
		"eventSubscriptions": map[string]any{
			"subscriptions": []any{
				map[string]any{
					"kind":             "erc20_transfer",
					"contractAddress":  "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
					"fromAddress":      "0xabc",
				},
			},
		},
	}
	p := resolvePipelineFilterPlan(cfg)
	if p.UseALLFallback {
		t.Fatalf("unexpected fallback: %q", p.Reason)
	}
	if p.EVMContract != "0x036cbd53842c5426634e7929541ec2318f3dcf7e" {
		t.Fatalf("contract: got %q", p.EVMContract)
	}
	if !p.EVMFromFilterUnmet {
		t.Fatal("expected EVMFromFilterUnmet")
	}
}

func TestResolvePipelineFilterPlanHederaPriorityOverEVM(t *testing.T) {
	t.Setenv("NARYO_ALLOW_ALL_BROADCASTER_FALLBACK", "false")
	cfg := map[string]any{
		"eventSubscriptions": map[string]any{
			"subscriptions": []any{
				map[string]any{"kind": "erc20_transfer", "contractAddress": "0x2222222222222222222222222222222222222222"},
				map[string]any{"kind": "hedera_hcs_topic", "topicId": "0.0.9"},
			},
		},
	}
	p := resolvePipelineFilterPlan(cfg)
	if p.HederaTopicID != "0.0.9" {
		t.Fatalf("want Hedera first, got %+v", p)
	}
}

func TestResolvePipelineFilterPlanEmptyConfigUsesALLWhenFallbackAllowed(t *testing.T) {
	t.Setenv("NARYO_ALLOW_ALL_BROADCASTER_FALLBACK", "true")
	t.Setenv("NARYO_DEFAULT_HCS_TOPIC_ID", "")
	p := resolvePipelineFilterPlan(nil)
	if !p.UseALLFallback || p.Key != "ALL" {
		t.Fatalf("got %+v", p)
	}
}

func TestDescribePipelineFilterPlanForSessionDemoShape(t *testing.T) {
	cfg := map[string]any{
		"eventSubscriptions": map[string]any{
			"subscriptions": []any{
				map[string]any{"kind": "hedera_hcs_topic", "topicId": "0.0.8510924", "hederaNetwork": "testnet"},
				map[string]any{"kind": "erc20_transfer", "contractAddress": "0x036CbD53842c5426634e7929541eC2318f3dCF7e"},
			},
		},
	}
	d := DescribePipelineFilterPlanForSession("sess-demo", cfg)
	if d["expectedNaryoFilterName"] != "pf-sess-demo-hcs" {
		t.Fatalf("expected HCS filter name first, got %#v", d["expectedNaryoFilterName"])
	}
	if d["useALLFallback"] != false {
		t.Fatalf("useALLFallback: got %#v", d["useALLFallback"])
	}
}

func TestDescribePipelineFilterPlanForSessionALL(t *testing.T) {
	t.Setenv("NARYO_ALLOW_ALL_BROADCASTER_FALLBACK", "true")
	t.Setenv("NARYO_DEFAULT_HCS_TOPIC_ID", "")
	d := DescribePipelineFilterPlanForSession("sess-x", map[string]any{})
	if d["useALLFallback"] != true {
		t.Fatal("expected ALL")
	}
	if _, ok := d["filterRowsNote"].(string); !ok {
		t.Fatal("expected filterRowsNote")
	}
}

func TestResolvePipelineFilterPlanEmptyConfigErrorWhenFallbackDisabled(t *testing.T) {
	t.Setenv("NARYO_ALLOW_ALL_BROADCASTER_FALLBACK", "false")
	t.Setenv("NARYO_DEFAULT_HCS_TOPIC_ID", "")
	p := resolvePipelineFilterPlan(map[string]any{})
	if p.UseALLFallback || p.HederaTopicID != "" || p.EVMContract != "" {
		t.Fatalf("got %+v", p)
	}
	if p.Reason == "" {
		t.Fatal("expected Reason")
	}
}
