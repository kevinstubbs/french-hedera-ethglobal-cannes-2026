package pipeline

import "context"

type mockHCS struct {
	summaries int
}

func (m *mockHCS) PipelineCreated(context.Context, string, string) {}
func (m *mockHCS) PipelineStarted(context.Context, string, string, string) {
}
func (m *mockHCS) PipelinePaused(context.Context, string, string, int64) {}
func (m *mockHCS) PipelineResumed(context.Context, string)              {}
func (m *mockHCS) PipelineStopped(context.Context, string)             {}
func (m *mockHCS) PipelineReconfigured(context.Context, string, int) {}
func (m *mockHCS) BillingTick(context.Context, string, int64, int64) {}
func (m *mockHCS) BillingSummary(context.Context, string, BillingSummaryArgs) {
	if m != nil {
		m.summaries++
	}
}
func (m *mockHCS) AgentTopUp(context.Context, AgentTopUpArgs) {}
func (m *mockHCS) StartRejectedInsufficientPrepaid(context.Context, string, string, int64, int64) {
}
func (m *mockHCS) PaymentStreamStarted(context.Context, string)    {}
func (m *mockHCS) PaymentStreamStalled(context.Context, string)   {}
func (m *mockHCS) PaymentStreamTerminated(context.Context, string) {}
