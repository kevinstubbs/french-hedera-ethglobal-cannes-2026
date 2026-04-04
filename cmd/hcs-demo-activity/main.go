// Command hcs-demo-activity submits HCS topic messages on a schedule for live demos and
// Naryo mirror indexing (when LOCAL_NODE_HCS_TOPIC_ID / legacy SOLO_HCS_TOPIC_ID / a Hedera filter targets the same topic).
//
// Env (required): HEDERA_OPERATOR_ID, HEDERA_OPERATOR_KEY — or RELAY_OPERATOR_ID_MAIN / RELAY_OPERATOR_KEY_MAIN
// (hedera-local-node .env). Loads ./.env and ../cmd/hcs-demo-activity/.env when vars are unset (does not override
// the real environment). Shell tip: `source .env` only exports if lines use `export KEY=...` or run `set -a; source .env; set +a`.
// Topic: -topic 0.0.n must be an existing consensus topic on this ledger (create one first with -create-topic).
// Env fallbacks: HEDERA_HCS_TOPIC_ID, LOCAL_NODE_HCS_TOPIC_ID, SOLO_HCS_TOPIC_ID, HEDERA_AUDIT_TOPIC_ID (first non-empty; not 0.0.0)
//
// Network: HEDERA_NETWORK=testnet|previewnet|mainnet|local (default testnet).
// Local defaults match hiero ClientForName("local"): gRPC 127.0.0.1:50211, mirror 127.0.0.1:5600.
// Override with HEDERA_LOCAL_GRPC, HEDERA_LOCAL_NODE_ACCOUNT_ID (default 0.0.3), HEDERA_LOCAL_MIRROR (comma-separated).
// For plaintext local gRPC, set HEDERA_LOCAL_PLAINTEXT=1.
// Optional: HEDERA_GRPC_DEADLINE (e.g. 2m) — default 2m to reduce DeadlineExceeded on long demo runs.
// Optional: MIRROR_REST_URL (default http://127.0.0.1:5551) — printed on first submit for mirror tx lookup hints.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	hiero "github.com/hiero-ledger/hiero-sdk-go/v2/sdk"
)

func main() {
	log.SetFlags(0)

	loadEnvFiles()

	var interval time.Duration
	flag.DurationVar(&interval, "interval", 5*time.Second, "delay between messages when not using -once")
	topicFlag := flag.String("topic", "", "existing HCS topic id 0.0.n (overrides env)")
	createTopic := flag.Bool("create-topic", false, "create a new consensus topic, print its id, and exit (use that id with -topic)")
	once := flag.Bool("once", false, "send one message and exit")
	flag.Parse()

	opID := strings.TrimSpace(firstNonEmpty(os.Getenv("HEDERA_OPERATOR_ID"), os.Getenv("RELAY_OPERATOR_ID_MAIN")))
	opKey := strings.TrimSpace(firstNonEmpty(os.Getenv("HEDERA_OPERATOR_KEY"), os.Getenv("RELAY_OPERATOR_KEY_MAIN")))
	if opID == "" || opKey == "" {
		log.Fatal("set HEDERA_OPERATOR_ID and HEDERA_OPERATOR_KEY (or RELAY_OPERATOR_* from hedera-local-node), " +
			"or put them in .env next to this command; plain `source .env` without `export` does not pass vars to go run")
	}

	client, err := dialClient()
	if err != nil {
		log.Fatalf("hedera client: %v", err)
	}
	defer client.Close()

	operator, err := hiero.AccountIDFromString(opID)
	if err != nil {
		log.Fatalf("operator id: %v", err)
	}
	key, err := hiero.PrivateKeyFromString(opKey)
	if err != nil {
		log.Fatalf("operator key: %v", err)
	}
	client.SetOperator(operator, key)
	applyClientDeadline(client)

	if *createTopic {
		tid, err := createConsensusTopic(client)
		if err != nil {
			log.Fatalf("create topic: %v", err)
		}
		log.Printf("hcs-demo-activity: created topic %s", tid)
		log.Printf("use: go run . -topic %s -interval 5s   (and set LOCAL_NODE_HCS_TOPIC_ID=%s for Naryo compose)", tid, tid)
		return
	}

	topicID := strings.TrimSpace(*topicFlag)
	if topicID == "" {
		topicID = firstNonEmpty(os.Getenv("HEDERA_HCS_TOPIC_ID"), os.Getenv("LOCAL_NODE_HCS_TOPIC_ID"), os.Getenv("SOLO_HCS_TOPIC_ID"), os.Getenv("HEDERA_AUDIT_TOPIC_ID"))
	}
	if topicID == "" || topicID == "0.0.0" {
		log.Fatal("topic required: use -topic with an existing topic, or run with -create-topic first; " +
			"or set HEDERA_HCS_TOPIC_ID / LOCAL_NODE_HCS_TOPIC_ID / SOLO_HCS_TOPIC_ID / HEDERA_AUDIT_TOPIC_ID")
	}

	topic, err := hiero.TopicIDFromString(topicID)
	if err != nil {
		log.Fatalf("topic id: %v", err)
	}

	seq := 0
	for {
		seq++
		payload, err := demoPayload(seq)
		if err != nil {
			log.Fatalf("payload: %v", err)
		}
		resp, err := hiero.NewTopicMessageSubmitTransaction().
			SetTopicID(topic).
			SetMessage(payload).
			Execute(client)
		if err != nil {
			if strings.Contains(err.Error(), "INVALID_TOPIC_ID") {
				log.Fatalf("submit: %v — topic does not exist on this ledger; run: go run . -create-topic  then use the printed id with -topic", err)
			}
			log.Fatalf("submit: %v", err)
		}
		txID := resp.TransactionID.String()
		log.Printf("hcs-demo-activity: submitted seq=%d tx=%s bytes=%d", seq, txID, len(payload))
		if seq == 1 {
			if p := mirrorRESTTransactionPath(resp.TransactionID); p != "" {
				base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("MIRROR_REST_URL")), "/")
				if base == "" {
					base = "http://127.0.0.1:5551"
				}
				log.Printf("hcs-demo-activity: mirror lookup %s/api/v1/transactions/%s (404 ⇒ mirror at %s is not ingesting txs from this consensus node)", base, p, base)
			}
		}
		if *once {
			return
		}
		time.Sleep(interval)
	}
}

// mirrorRESTTransactionPath builds the {transactionId} path segment for Hiero mirror REST (GET /api/v1/transactions/{id}).
func mirrorRESTTransactionPath(id hiero.TransactionID) string {
	if id.AccountID == nil || id.ValidStart == nil {
		return ""
	}
	t := *id.ValidStart
	return fmt.Sprintf("%s-%d-%09d", id.AccountID.String(), t.Unix(), t.Nanosecond())
}

func applyClientDeadline(client *hiero.Client) {
	if client == nil {
		return
	}
	d := 2 * time.Minute
	if s := strings.TrimSpace(os.Getenv("HEDERA_GRPC_DEADLINE")); s != "" {
		if parsed, err := time.ParseDuration(s); err == nil && parsed > 0 {
			d = parsed
		}
	}
	client.SetGrpcDeadline(d)
}

func createConsensusTopic(client *hiero.Client) (hiero.TopicID, error) {
	resp, err := hiero.NewTopicCreateTransaction().Execute(client)
	if err != nil {
		return hiero.TopicID{}, err
	}
	receipt, err := resp.SetValidateStatus(true).GetReceipt(client)
	if err != nil {
		return hiero.TopicID{}, err
	}
	if receipt.TopicID == nil {
		return hiero.TopicID{}, fmt.Errorf("topic create: missing topic id in receipt")
	}
	return *receipt.TopicID, nil
}

// loadEnvFiles reads dotenv-style files and sets keys only when not already in the process environment.
func loadEnvFiles() {
	for _, path := range []string{".env", "cmd/hcs-demo-activity/.env"} {
		_ = loadDotEnv(path)
	}
}

func loadDotEnv(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		if os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, val)
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func dialClient() (*hiero.Client, error) {
	net := strings.ToLower(strings.TrimSpace(os.Getenv("HEDERA_NETWORK")))
	if net == "" {
		net = "testnet"
	}

	localGRPC := strings.TrimSpace(os.Getenv("HEDERA_LOCAL_GRPC"))
	if localGRPC != "" {
		nodeAcc := strings.TrimSpace(os.Getenv("HEDERA_LOCAL_NODE_ACCOUNT_ID"))
		if nodeAcc == "" {
			nodeAcc = "0.0.3"
		}
		acc, err := hiero.AccountIDFromString(nodeAcc)
		if err != nil {
			return nil, fmt.Errorf("HEDERA_LOCAL_NODE_ACCOUNT_ID: %w", err)
		}
		c, err := hiero.ClientForNetworkV2(map[string]hiero.AccountID{localGRPC: acc})
		if err != nil {
			return nil, err
		}
		mirrors := parseMirrorList(os.Getenv("HEDERA_LOCAL_MIRROR"))
		if len(mirrors) > 0 {
			c.SetMirrorNetwork(mirrors)
		}
		if truthy(os.Getenv("HEDERA_LOCAL_PLAINTEXT")) {
			c.SetTransportSecurity(false)
		}
		return c, nil
	}

	switch net {
	case "local", "localhost":
		c, err := hiero.ClientForName("local")
		if err != nil {
			return nil, err
		}
		if truthy(os.Getenv("HEDERA_LOCAL_PLAINTEXT")) {
			c.SetTransportSecurity(false)
		}
		return c, nil
	case "mainnet":
		return hiero.ClientForMainnet(), nil
	case "previewnet":
		return hiero.ClientForPreviewnet(), nil
	default:
		return hiero.ClientForTestnet(), nil
	}
}

func parseMirrorList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func truthy(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes"
}

func demoPayload(seq int) ([]byte, error) {
	m := map[string]any{
		"demo":      "hcs-demo-activity",
		"seq":       seq,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"pid":       os.Getpid(),
	}
	return json.Marshal(m)
}
