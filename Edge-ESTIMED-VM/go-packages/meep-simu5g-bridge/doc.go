/*
 * Copyright (c) 2026  The AdvantEDGE Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 */

/**
 * @file doc.go
 * @brief Shared Simu5G integration library for two-host MEC Sandbox deployment.
 *
 * @details
 * Host **B** (sandbox) runs meep-simu5g-bridge, mobility adapter, and RNIS adapter.
 * Host **S** (simulator) runs OMNeT++/Simu5G with TCP bridge and UDP responders.
 *
 * This package provides:
 * - JSON integration configuration (@ref IntegrationConfig)
 * - TCP bridge framing for user-plane IP (@ref EncodeDataULFrame, @ref PacketFromDataDLFrame)
 * - UE telemetry UDP parsing from S→B (@ref ParseTelemetryDatagram)
 * - RNI query/response UDP codec for B→S (@ref QueryRNIUDP)
 *
 * @see docs/Simu5G/interfaces/bridge-protocol.md
 * @see docs/Simu5G/two-host-deployment.md
 *
 * @defgroup simu5g_bridge_pkg meep-simu5g-bridge (library)
 * @{
 */
package bridge
