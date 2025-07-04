// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cfgplugins

import (
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/openconfig/featureprofiles/internal/components"
	"github.com/openconfig/featureprofiles/internal/deviations"
	"github.com/openconfig/featureprofiles/internal/fptest"
	"github.com/openconfig/ondatra"
	"github.com/openconfig/ondatra/gnmi"
	"github.com/openconfig/ondatra/gnmi/oc"
	"github.com/openconfig/ygot/ygot"
)

const (
	targetOutputPowerdBm          = -10
	targetOutputPowerTolerancedBm = 1
	targetFrequencyMHz            = 193100000
	targetFrequencyToleranceMHz   = 100000
)

var (
	opmode uint16
	once   sync.Once
)

// Temporary code for assigning opmode 1 maintained until opmode is Initialized in all .go file
func init() {
	opmode = 1
}

// InterfaceInitialize assigns OpMode with value received through operationalMode flag.
func InterfaceInitialize(t *testing.T, dut *ondatra.DUTDevice, initialOperationalMode uint16) uint16 {
	once.Do(func() {
		t.Helper()
		if initialOperationalMode == 0 { // '0' signals to use vendor-specific default
			switch dut.Vendor() {
			case ondatra.CISCO:
				opmode = 5003
				t.Logf("cfgplugins.Initialize: Cisco DUT, setting opmode to default: %d", opmode)
			case ondatra.ARISTA:
				opmode = 1
				t.Logf("cfgplugins.Initialize: Arista DUT, setting opmode to default: %d", opmode)
			case ondatra.JUNIPER:
				opmode = 1
				t.Logf("cfgplugins.Initialize: Juniper DUT, setting opmode to default: %d", opmode)
			case ondatra.NOKIA:
				opmode = 1083
				t.Logf("cfgplugins.Initialize: Nokia DUT, setting opmode to default: %d", opmode)
			default:
				opmode = 1
				t.Logf("cfgplugins.Initialize: Using global default opmode: %d", opmode)
			}
		} else {
			opmode = initialOperationalMode
			t.Logf("cfgplugins.Initialize: Using provided initialOperationalMode: %d", opmode)
		}
		t.Logf("cfgplugins.Initialize: Initialization complete. Final opmode set to: %d", opmode)
	})
	return InterfaceGetOpMode()
}

// InterfaceGetOpMode returns the opmode value after the Initialize function has been called
func InterfaceGetOpMode() uint16 {
	return opmode
}

// InterfaceConfig configures the interface with the given port.
func InterfaceConfig(t *testing.T, dut *ondatra.DUTDevice, dp *ondatra.Port) {
	t.Helper()
	d := &oc.Root{}
	i := d.GetOrCreateInterface(dp.Name())
	i.Enabled = ygot.Bool(true)
	i.Type = oc.IETFInterfaces_InterfaceType_ethernetCsmacd
	gnmi.Replace(t, dut, gnmi.OC().Interface(dp.Name()).Config(), i)
	if deviations.ExplicitDcoConfig(dut) {
		transceiverName := gnmi.Get(t, dut, gnmi.OC().Interface(dp.Name()).Transceiver().State())
		gnmi.Replace(t, dut, gnmi.OC().Component(transceiverName).Config(), &oc.Component{
			Name: ygot.String(transceiverName),
			Transceiver: &oc.Component_Transceiver{
				ModuleFunctionalType: oc.TransportTypes_TRANSCEIVER_MODULE_FUNCTIONAL_TYPE_TYPE_DIGITAL_COHERENT_OPTIC,
			},
		})
	}
	oc := components.OpticalChannelComponentFromPort(t, dut, dp)
	ConfigOpticalChannel(t, dut, oc, targetFrequencyMHz, targetOutputPowerdBm, opmode)
}

// ValidateInterfaceConfig validates the output power and frequency for the given port.
func ValidateInterfaceConfig(t *testing.T, dut *ondatra.DUTDevice, dp *ondatra.Port, targetOutputPowerdBm float64, targetFrequencyMHz uint64, targetOutputPowerTolerancedBm float64, targetFrequencyToleranceMHz float64) {
	t.Helper()
	ocComponent := components.OpticalChannelComponentFromPort(t, dut, dp)
	t.Logf("Got opticalChannelComponent from port: %s", ocComponent)

	outputPower := gnmi.Get(t, dut, gnmi.OC().Component(ocComponent).OpticalChannel().TargetOutputPower().State())
	if math.Abs(float64(outputPower)-float64(targetOutputPowerdBm)) > targetOutputPowerTolerancedBm {
		t.Fatalf("Output power is not within expected tolerance, got: %v want: %v tolerance: %v", outputPower, targetOutputPowerdBm, targetOutputPowerTolerancedBm)
	}

	frequency := gnmi.Get(t, dut, gnmi.OC().Component(ocComponent).OpticalChannel().Frequency().State())
	if math.Abs(float64(frequency)-float64(targetFrequencyMHz)) > targetFrequencyToleranceMHz {
		t.Fatalf("Frequency is not within expected tolerance, got: %v want: %v tolerance: %v", frequency, targetFrequencyMHz, targetFrequencyToleranceMHz)
	}
}

// ToggleInterface toggles the interface.
func ToggleInterface(t *testing.T, dut *ondatra.DUTDevice, intf string, isEnabled bool) {
	d := &oc.Root{}
	i := d.GetOrCreateInterface(intf)
	i.Type = oc.IETFInterfaces_InterfaceType_ethernetCsmacd
	i.Enabled = ygot.Bool(isEnabled)
	gnmi.Replace(t, dut, gnmi.OC().Interface(intf).Config(), i)
}

// ConfigOpticalChannel configures the optical channel.
func ConfigOpticalChannel(t *testing.T, dut *ondatra.DUTDevice, och string, frequency uint64, targetOpticalPower float64, operationalMode uint16) {
	gnmi.Replace(t, dut, gnmi.OC().Component(och).Config(), &oc.Component{
		Name: ygot.String(och),
		OpticalChannel: &oc.Component_OpticalChannel{
			OperationalMode:   ygot.Uint16(operationalMode),
			Frequency:         ygot.Uint64(frequency),
			TargetOutputPower: ygot.Float64(targetOpticalPower),
		},
	})
}

// ConfigOTNChannel configures the OTN channel.
func ConfigOTNChannel(t *testing.T, dut *ondatra.DUTDevice, och string, otnIndex, ethIndex uint32) {
	t.Helper()
	t.Logf(" otnIndex:%v, ethIndex: %v", otnIndex, ethIndex)
	if deviations.OTNChannelTribUnsupported(dut) {
		gnmi.Replace(t, dut, gnmi.OC().TerminalDevice().Channel(otnIndex).Config(), &oc.TerminalDevice_Channel{
			Description:        ygot.String("OTN Logical Channel"),
			Index:              ygot.Uint32(otnIndex),
			LogicalChannelType: oc.TransportTypes_LOGICAL_ELEMENT_PROTOCOL_TYPE_PROT_OTN,
			Assignment: map[uint32]*oc.TerminalDevice_Channel_Assignment{
				0: {
					Index:          ygot.Uint32(1),
					OpticalChannel: ygot.String(och),
					Description:    ygot.String("OTN to Optical Channel"),
					Allocation:     ygot.Float64(400),
					AssignmentType: oc.Assignment_AssignmentType_OPTICAL_CHANNEL,
				},
			},
		})
	} else {
		gnmi.Replace(t, dut, gnmi.OC().TerminalDevice().Channel(otnIndex).Config(), &oc.TerminalDevice_Channel{
			Description:        ygot.String("OTN Logical Channel"),
			Index:              ygot.Uint32(otnIndex),
			LogicalChannelType: oc.TransportTypes_LOGICAL_ELEMENT_PROTOCOL_TYPE_PROT_OTN,
			TribProtocol:       oc.TransportTypes_TRIBUTARY_PROTOCOL_TYPE_PROT_400GE,
			AdminState:         oc.TerminalDevice_AdminStateType_ENABLED,
			Assignment: map[uint32]*oc.TerminalDevice_Channel_Assignment{
				0: {
					Index:          ygot.Uint32(0),
					OpticalChannel: ygot.String(och),
					Description:    ygot.String("OTN to Optical Channel"),
					Allocation:     ygot.Float64(400),
					AssignmentType: oc.Assignment_AssignmentType_OPTICAL_CHANNEL,
				},
			},
		})
	}
}

// ConfigETHChannel configures the ETH channel.
func ConfigETHChannel(t *testing.T, dut *ondatra.DUTDevice, interfaceName, transceiverName string, otnIndex, ethIndex uint32) {
	t.Helper()
	var ingress = &oc.TerminalDevice_Channel_Ingress{}
	if !deviations.EthChannelIngressParametersUnsupported(dut) {
		ingress = &oc.TerminalDevice_Channel_Ingress{
			Interface:   ygot.String(interfaceName),
			Transceiver: ygot.String(transceiverName),
		}
	}
	var assignment = map[uint32]*oc.TerminalDevice_Channel_Assignment{
		0: {
			Index:          ygot.Uint32(0),
			LogicalChannel: ygot.Uint32(otnIndex),
			Description:    ygot.String("ETH to OTN"),
			Allocation:     ygot.Float64(400),
			AssignmentType: oc.Assignment_AssignmentType_LOGICAL_CHANNEL,
		},
	}
	if deviations.EthChannelAssignmentCiscoNumbering(dut) {
		assignment[0].Index = ygot.Uint32(1)
	}
	var channel = &oc.TerminalDevice_Channel{
		Description:        ygot.String("ETH Logical Channel"),
		Index:              ygot.Uint32(ethIndex),
		LogicalChannelType: oc.TransportTypes_LOGICAL_ELEMENT_PROTOCOL_TYPE_PROT_ETHERNET,
		TribProtocol:       oc.TransportTypes_TRIBUTARY_PROTOCOL_TYPE_PROT_400GE,
		Ingress:            ingress,
		Assignment:         assignment,
		AdminState:         oc.TerminalDevice_AdminStateType_ENABLED,
	}
	if !deviations.ChannelRateClassParametersUnsupported(dut) {
		channel.RateClass = oc.TransportTypes_TRIBUTARY_RATE_CLASS_TYPE_TRIB_RATE_400G
	}
	gnmi.Replace(t, dut, gnmi.OC().TerminalDevice().Channel(ethIndex).Config(), channel)
}

// SetupAggregateAtomically sets up the aggregate interface atomically.
func SetupAggregateAtomically(t *testing.T, dut *ondatra.DUTDevice, aggID string, dutAggPorts []*ondatra.Port) {
	d := &oc.Root{}

	d.GetOrCreateLacp().GetOrCreateInterface(aggID)

	agg := d.GetOrCreateInterface(aggID)
	agg.GetOrCreateAggregation().LagType = oc.IfAggregate_AggregationType_LACP
	agg.Type = ieee8023adLag

	for _, port := range dutAggPorts {
		i := d.GetOrCreateInterface(port.Name())
		i.GetOrCreateEthernet().AggregateId = ygot.String(aggID)
		i.Type = ethernetCsmacd

		if deviations.InterfaceEnabled(dut) {
			i.Enabled = ygot.Bool(true)
		}
	}

	p := gnmi.OC()
	fptest.LogQuery(t, fmt.Sprintf("%s to Update()", dut), p.Config(), d)
	gnmi.Update(t, dut, p.Config(), d)
}

// DeleteAggregate deletes the aggregate interface.
func DeleteAggregate(t *testing.T, dut *ondatra.DUTDevice, aggID string, dutAggPorts []*ondatra.Port) {
	// Clear the aggregate minlink.
	gnmi.Delete(t, dut, gnmi.OC().Interface(aggID).Aggregation().MinLinks().Config())

	// Clear the members of the aggregate.
	for _, port := range dutAggPorts {
		gnmi.Delete(t, dut, gnmi.OC().Interface(port.Name()).Ethernet().AggregateId().Config())
	}
}
