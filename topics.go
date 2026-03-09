package main

type Topic struct {
	Name  string `json:"name"`
	Scale int    `json:"scale"`
	Unit  string `json:"unit"`
}

var TopicRegistry = map[string]Topic{
	// Scale 100 — floats published with 2 decimal places
	"ac_input_power":    {Name: "AC Input Power", Scale: 100, Unit: "W"},
	"ac_output_power":   {Name: "AC Output Power", Scale: 100, Unit: "W"},
	"dc_input_power":    {Name: "DC Input Power", Scale: 100, Unit: "W"},
	"dc12v_output_power": {Name: "DC 12V Output Power", Scale: 100, Unit: "W"},
	"usba_output_power": {Name: "USB-A Output Power", Scale: 100, Unit: "W"},
	"usbc_output_power": {Name: "USB-C Output Power", Scale: 100, Unit: "W"},
	"cell_temperature":  {Name: "Cell Temperature", Scale: 100, Unit: "°C"},
	"input_power":       {Name: "Total Input Power", Scale: 100, Unit: "W"},
	"output_power":      {Name: "Total Output Power", Scale: 100, Unit: "W"},
	"battery_level":     {Name: "Battery Level", Scale: 100, Unit: "%"},

	// Scale 1 — integers
	"battery_charge_limit_min": {Name: "Charge Limit Min", Scale: 1, Unit: "%"},
	"battery_charge_limit_max": {Name: "Charge Limit Max", Scale: 1, Unit: "%"},
	"ac_charging_speed":        {Name: "AC Charging Speed", Scale: 1, Unit: ""},
	"remaining_time_charging":    {Name: "Time to Full", Scale: 1, Unit: "min"},
	"remaining_time_discharging": {Name: "Time to Empty", Scale: 1, Unit: "min"},
	"plugged_in_ac":  {Name: "AC Plugged In", Scale: 1, Unit: ""},
	"energy_backup":  {Name: "Energy Backup", Scale: 1, Unit: ""},
	"online":         {Name: "Online", Scale: 1, Unit: ""},
}
