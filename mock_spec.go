package main

type LDAPMock struct {
	Users []User `yaml:"users"`
}

type User struct {
	CN    string            `yaml:"cn"`
	Attrs map[string]string `yaml:"attrs"`
}
