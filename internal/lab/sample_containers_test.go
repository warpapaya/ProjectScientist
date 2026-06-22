package lab

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSampleCreatePersistsContainersAliquotsAndPartitions(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := testActor("container-accessioner")
	client, _ := store.CreateClient("Container Client", "containers@example.test", actor)
	containerRef, _ := store.CreateSampleReferenceItem(SampleReferenceItemInput{Kind: SampleReferenceContainer, Code: "HDPE", Name: "HDPE Bottle", Active: true}, actor)
	preservative, _ := store.CreateSampleReferenceItem(SampleReferenceItemInput{Kind: SampleReferencePreservative, Code: "HNO3", Name: "Nitric Acid", Active: true}, actor)
	condition, _ := store.CreateSampleReferenceItem(SampleReferenceItemInput{Kind: SampleReferenceReceivedCondition, Code: "OK", Name: "Intact on ice", Active: true}, actor)
	dept, _ := store.CreateCatalogDepartment(CatalogDepartmentInput{Name: "Metals", SortOrder: 1}, actor)
	method, _ := store.CreateCatalogMethod(CatalogMethodInput{Name: "EPA 200.8"}, actor)

	sample, err := store.CreateSample(CreateSampleInput{
		ClientID: client.ID,
		Project:  "Container Modeling",
		Matrix:   "Water",
		Tests:    []string{"Lead"},
		Containers: []SampleContainerInput{{
			ContainerReferenceID: containerRef.ID,
			PreservativeID:       preservative.ID,
			ReceivedConditionID:  condition.ID,
			Volume:               "500 mL",
			Condition:            "intact seal",
			AliquotInstructions:  "split for dissolved metals",
			Aliquots: []SampleAliquotInput{{
				DepartmentID: dept.ID,
				MethodID:     method.ID,
				Volume:       "125 mL",
				Purpose:      "dissolved metals prep",
			}},
		}},
	}, actor)
	if err != nil {
		t.Fatalf("create sample with containers: %v", err)
	}
	if len(sample.Containers) != 1 {
		t.Fatalf("expected one physical container, got %#v", sample.Containers)
	}
	container := sample.Containers[0]
	if container.ID == "" || container.SampleID != sample.ID || container.ContainerReferenceID != containerRef.ID || container.PreservativeID != preservative.ID || container.ReceivedConditionID != condition.ID || container.Volume != "500 mL" || container.Condition != "intact seal" {
		t.Fatalf("container relationship/metadata not persisted: %#v", container)
	}
	if len(container.Aliquots) != 1 || container.Aliquots[0].DepartmentID != dept.ID || container.Aliquots[0].DepartmentName != "Metals" || container.Aliquots[0].MethodID != method.ID || container.Aliquots[0].MethodName != "EPA 200.8" || container.Aliquots[0].Purpose != "dissolved metals prep" {
		t.Fatalf("aliquot partition metadata not persisted: %#v", container.Aliquots)
	}

	loaded, ok := store.GetSample(sample.ID)
	if !ok {
		t.Fatalf("load sample %q", sample.ID)
	}
	if len(loaded.Containers) != 1 || len(loaded.Containers[0].Aliquots) != 1 || loaded.Containers[0].Aliquots[0].DepartmentName != "Metals" {
		t.Fatalf("loaded sample lost container/aliquot model: %#v", loaded.Containers)
	}
}

func TestSampleContainerMutationsAreCustodySafe(t *testing.T) {
	store, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "project-scientist.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	actor := testActor("custody-accessioner")
	client, _ := store.CreateClient("Custody Client", "custody@example.test", actor)
	containerRef, _ := store.CreateSampleReferenceItem(SampleReferenceItemInput{Kind: SampleReferenceContainer, Code: "VOA", Name: "VOA vial", Active: true}, actor)
	sample, err := store.CreateSample(CreateSampleInput{ClientID: client.ID, Project: "Custody", Matrix: "Water", Tests: []string{"VOC"}}, actor)
	if err != nil {
		t.Fatalf("create sample: %v", err)
	}
	if err := store.TransitionSample(sample.ID, StatusInPrep, actor); err != nil {
		t.Fatalf("transition to prep: %v", err)
	}
	if _, err := store.AddSampleContainer(sample.ID, SampleContainerInput{ContainerReferenceID: containerRef.ID, Volume: "40 mL"}, actor); err == nil || !strings.Contains(err.Error(), "received samples") {
		t.Fatalf("expected custody-safe mutation denial for in-prep sample, got %v", err)
	}
	loaded, _ := store.GetSample(sample.ID)
	if len(loaded.Containers) != 0 {
		t.Fatalf("denied container mutation should not alter sample: %#v", loaded.Containers)
	}
}
