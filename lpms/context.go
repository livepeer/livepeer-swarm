package lpms

import "context"

//The LPMS context contains config information for LPMS.
type Context struct {
	config        *Config
	parentContext *context.Context
}
