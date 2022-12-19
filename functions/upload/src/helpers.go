package upload

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"os"
)

const (
	defaultImportRole = "vmimport"
	importRoleEnv     = "VM_IMPORT_ROLE_NAME"
)

func roleName() *string {
	if role, ok := os.LookupEnv(importRoleEnv); ok {
		return aws.String(role)
	}
	return aws.String(defaultImportRole)
}
