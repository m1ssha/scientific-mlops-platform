// Command fixture creates a disposable catalog for the production browser smoke.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/m1ssha/scientific-mlops-core/catalog"
	"github.com/m1ssha/scientific-mlops-core/object"
	"github.com/m1ssha/scientific-mlops-core/storage/local"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		panic("fixture requires an empty directory")
	}
	ctx := context.Background()
	root := os.Args[1]
	const repo catalog.ID = "00000000000000000000000000000001"
	m, e := catalog.NewLocalMetadata(root)
	check(e)
	check(m.InitRepository(ctx, repo))
	o, e := local.New(root)
	check(e)
	data := []byte("test")
	hash := sha256.Sum256(data)
	ref := object.Ref{Algorithm: "sha256", Digest: fmt.Sprintf("%x", hash), Size: int64(len(data))}
	_, e = o.Put(ctx, ref, bytes.NewReader(data))
	check(e)
	s := catalog.NewService(m, o)
	d, e := s.PublishDataset(ctx, catalog.Command{SchemaVersion: 1, RepositoryID: repo, OperationID: "00000000000000000000000000000002", Type: "publish_dataset", Body: catalog.DatasetBody{Files: []catalog.DatasetFile{{Path: "input", Object: ref}}}})
	check(e)
	mat := func(name, purpose, path string) catalog.Material {
		return catalog.Material{Name: name, Purpose: purpose, Required: true, Path: path, Source: catalog.MaterialSource{Kind: "object", Object: &ref}}
	}
	body := catalog.SnapshotBody{ProfileID: "profile", Inputs: []catalog.SnapshotInput{{Role: "train", DatasetID: *d.RecordID, TargetPrefix: "data"}}, Code: catalog.Code{Repository: "example", Commit: "1111111111111111111111111111111111111111", EntryPoint: "src/main"}, Parameters: map[string]catalog.TypedValue{"seed": {Type: "integer", Value: int64(1)}}, Split: catalog.Split{Description: "split", Materials: []string{"split"}}, Environment: catalog.Environment{Description: "env", Materials: []string{"env"}, Conditions: map[string]catalog.TypedValue{"version": {Type: "number", Value: float64(1)}}}, Materials: []catalog.Material{mat("code", "code", "src/main"), mat("split", "split", "split"), mat("env", "environment", "env")}}
	snap, e := s.SaveSnapshot(ctx, catalog.Command{SchemaVersion: 1, RepositoryID: repo, OperationID: "00000000000000000000000000000003", Type: "save_snapshot", Body: body})
	check(e)
	evidence := "reported"
	run := catalog.RunBody{SnapshotID: *snap.RecordID, Outcome: "succeeded", Observations: map[string]catalog.Observation{"version": {Status: "passed", Value: &catalog.TypedValue{Type: "number", Value: float64(1)}, Evidence: &evidence}}, Metrics: map[string]catalog.Metric{}, Outputs: []catalog.Output{}, EvidenceSource: "caller_reported"}
	for _, id := range []catalog.ID{"00000000000000000000000000000004", "00000000000000000000000000000005"} {
		_, e = s.RegisterRun(ctx, catalog.Command{SchemaVersion: 1, RepositoryID: repo, OperationID: id, Type: "register_run", Body: run})
		check(e)
	}
}
func check(e error) {
	if e != nil {
		panic(e)
	}
}
