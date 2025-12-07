package main

type LDAPMock struct {
	Users []User `yaml:"users"`
	Rules []Rule `yaml:"rules"`
}

type User struct {
	CN    string            `yaml:"cn"`
	Attrs map[string]string `yaml:"attrs"`
}

type Group struct {
	CN      string            `yaml:"cn"`
	Members []string          `yaml:"members"`
	Attrs   map[string]string `yaml:"attrs"`
}

type Rule struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Filter   string   `yaml:"filter"`
	BaseDN   string   `yaml:"base_dn"`
	Scope    string   `yaml:"scope"`
	Priority int      `yaml:"priority"`
	Response Response `yaml:"response"`
}

type Response struct {
	Users  []User  `yaml:"users"`
	Groups []Group `yaml:"groups"`
}
