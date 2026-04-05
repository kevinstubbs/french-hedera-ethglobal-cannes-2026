package naryo

// EnsurePipelineArgs is passed to [Client.EnsurePipeline] and [Client.ResumeEgress] so Naryo can
// create a pipeline-scoped filter from session.Config (e.g. eventSubscriptions).
type EnsurePipelineArgs struct {
	SessionID string
	Config    map[string]any
}
