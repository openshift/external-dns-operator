package v1beta1

import (
	"context"
	"fmt"
	"testing"
)

func TestValidatorDelegates(test *testing.T) {
	oldObject := &ExternalDNS{}
	oldObject.Spec.Source.Type = SourceTypeCRD
	newObject := &ExternalDNS{}
	validator := externalDNSValidator{}
	for _, operation := range []string{"create", "update", "delete"} {
		test.Run(operation, func(test *testing.T) {
			var expected, actual error
			switch operation {
			case "create":
				_, expected = newObject.ValidateCreate()
				_, actual = validator.ValidateCreate(context.Background(), newObject)
			case "update":
				_, expected = newObject.ValidateUpdate(oldObject)
				_, actual = validator.ValidateUpdate(context.Background(), oldObject, newObject)
			case "delete":
				_, expected = newObject.ValidateDelete()
				_, actual = validator.ValidateDelete(context.Background(), newObject)
			}
			if fmt.Sprint(actual) != fmt.Sprint(expected) {
				test.Fatalf("validation changed: got %v, want %v", actual, expected)
			}
		})
	}
}

func TestValidatorRejectsInvalidDomain(test *testing.T) {
	object := &ExternalDNS{}
	object.Spec.Domains = []ExternalDNSDomain{{ExternalDNSDomainUnion: ExternalDNSDomainUnion{MatchType: DomainMatchTypeExact}}}
	validator := externalDNSValidator{}
	if _, err := validator.ValidateCreate(context.Background(), object); err == nil {
		test.Fatal("invalid create accepted")
	}
	if _, err := validator.ValidateUpdate(context.Background(), &ExternalDNS{}, object); err == nil {
		test.Fatal("invalid update accepted")
	}
	if _, err := validator.ValidateDelete(context.Background(), object); err != nil {
		test.Fatalf("delete unexpectedly validated the object: %v", err)
	}
}

func TestValidatorPreservesOldSource(test *testing.T) {
	for _, testCase := range []struct {
		name      string
		oldSource ExternalDNSSourceType
		newSource ExternalDNSSourceType
		wantError bool
	}{
		{name: "existing CRD source", oldSource: SourceTypeCRD, newSource: SourceTypeCRD},
		{name: "new CRD source", oldSource: SourceTypeRoute, newSource: SourceTypeCRD, wantError: true},
		{name: "remove CRD source", oldSource: SourceTypeCRD, newSource: SourceTypeRoute},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			oldObject, newObject := &ExternalDNS{}, &ExternalDNS{}
			oldObject.Spec.Source.Type = testCase.oldSource
			newObject.Spec.Source.Type = testCase.newSource
			_, err := (externalDNSValidator{}).ValidateUpdate(context.Background(), oldObject, newObject)
			if (err != nil) != testCase.wantError {
				test.Fatalf("update error=%v, want error=%v", err, testCase.wantError)
			}
		})
	}
}
