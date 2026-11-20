#
#	MECSupport.py
#
#	(c) 2026 by STF 685-ESTIMED
#	License: BSD 3-Clause License. See the LICENSE file for further details.
#
"""	Plugin to add the support of MN-CSE Registration to MEC. """

from __future__ import annotations

from typing import Tuple, cast, Dict, Optional, Any

import time
import flask
#from flask import Flask, Request, request
from flask import Flask

from acmecse.plugins.bindings.HttpServer import FlaskHandler
from acmecse.runtime import CSE
from acmecse.runtime.Logging import Logging as L
from acmecse.etc.Utils import normalizeURL
from acmecse.etc.Types import ResponseStatusCode, ContentSerializationType, AuthorizationResult
from acmecse.helpers.PluginManager import plugin, init, start, restart, stop, finish, configure, validate, requires
from acmecse.helpers.interpreter.Interpreter import SType
from acmecse.helpers.BackgroundWorker import BackgroundWorker, BackgroundWorkerPool
from acmecse.etc.ResponseStatusCodes import ResponseException
from acmecse.runtime.Configuration import Configuration
from acmecse.etc.Types import Result, ResponseStatusCode
from acmecse.runtime.Configuration import Configuration, ConfigurationError
from acmecse.runtime.EventManager import onEvent, EventData, eventManager, eventHandler

import requests
import isodate

from urllib.parse import urlparse, unquote

import uuid

import json
import threading

@eventHandler
@plugin(property='MECSupport', tags=['acme', 'core'])
@requires(cseShutdown='acmecse.runtime.CSE.shutdown')
class MECSupport:
	"""	Plugin class to add upport of MN-CSE Registration to MEC.
	"""

	__slots__ = ( # TODO 
		'http_address',
		'http_port',
		'cse_external_ip',
		'cse_external_port',
		'use_wss',
		'mqtt_address',
		'mqtt_port',
		'wss_path',
		'cse_resourceID',
		'fullAddress',
		'isStopped',
		'mec_app_instance_id',
		'mecConnection',
		'mecConnections',
		'isInfo',
		'headers',
		'register',
		'thread',
		'mecRegistrationWorker'
	)
	""" Slots for the MECSupport. """

	@init
	def initMECSupport(self) -> None:
		"""	Initialize the MEC support plugin.
		"""
		L.isDebug and L.logDebug('Initializing MN-CSE Registration to MEC plugin')
		self.isStopped = True
		""" Flag to indicate whether the MEC client is stopped. """
		self.headers = {}
		self.headers['Content-Type'] = 'application/json; charset=UTF-8'
		self.register = None
		self.thread = None
		self.mecRegistrationWorker = None
		self.use_wss = False
		self.wss_path = None
		L.isInfo and L.log('initMECSupport: MEC Client initialized')
		return

	@start
	def startMECSupport(self) -> None:
		L.isDebug and L.logDebug('startMECSupport: Starting MN-CSE Registration to MEC plugin')
		if not Configuration.mec_enable:
			L.isInfo and L.log('MEC Client disabled')
			return
		L.isInfo and L.log(f'startMECSupport: Call connectToMec with {self.http_address}')
		self.mecConnection = self.connectToMec(host = self.http_address, port = self.http_port)
		if self.mecConnection is None:
			L.logErr('startMECSupport: Failed to connect to MEC: self.mecConnection is None')
			return
		L.isInfo and L.log('Start MEC client')
		try:
			self.thread = threading.Thread(target=self.mecConnection.run, kwargs={'host': self.http_address, 'port': self.http_port, 'threaded': True}, daemon=True)
			# store references on the Flask app object so they can be accessed later (avoid adding to MECSupport __slots__)
			setattr(self.mecConnection, 'server_thread', self.thread)
			setattr(self.mecConnection, 'server_stop_event', threading.Event())
			self.thread.start()
			L.isInfo and L.log('startMECSupport: MEC Flask app running in background thread')
			self.isStopped = False
		except Exception as e:
			L.logErr(f'startMECSupport: failed to start MEC Flask thread: {e}')
		L.isInfo and L.log('<<< startMECSupport')
		return

	@onEvent(eventManager.cseStartup)
	def startMECRegistration(self, eventData: EventData) -> None:
		L.isDebug and L.logDebug('startMECRegistration: Initiating MN-CSE Registration to MEC on cseStartup event')
		if not Configuration.mec_enable or self.isStopped:
			L.isInfo and L.log('MEC Client disabled or stopped')
			return
		# Register to MEC platform
		self.registerToMECPlatform()
		L.isInfo and L.log('startMECRegistration: Starting MEC registration worker')
		self.mecRegistrationWorker = BackgroundWorkerPool.newWorker(20, # Don't care, the worker will be stopped by registerAsIoTPlatform() in any case
																	self.registerAsIoTPlatform, 
																	'MECSupport').start()
		return

	@finish
	def finishMECSupport(self) -> None:
		L.isDebug and L.logDebug('Finishing MN-CSE Registration to MEC plugin')
		if not Configuration.mec_enable:
			L.isInfo and L.log('MEC Client disabled')
			return
		if self.mecRegistrationWorker is not None:
			L.isInfo and L.log('finishMECSupport: Stopping MEC registration worker')
			self.mecRegistrationWorker.stop()
			self.mecRegistrationWorker = None
		for id in list(self.mecConnections):
			self.disconnectFromMec(id[0], id[1])	# 0 = host, 1 = port
		if self.thread and self.thread.is_alive():
			L.isInfo and L.log('finishMECSupport: Stopping MEC Flask app thread')
			# Signal the Flask server to stop
			getattr(self.mecConnection, 'server_stop_event').set()
			self.thread.join(timeout=5)
			if self.thread.is_alive():
				L.logErr('finishMECSupport: Failed to stop MEC Flask app thread within timeout')
		self.mecConnection = None
		self.isStopped = True
		L.isInfo and L.log('MEC client shut down')
		return

	@restart
	def restartMECSupport(self) -> None:
		L.isDebug and L.logDebug('Restarting MN-CSE Registration to MEC plugin')
		if not Configuration.mec_enable:
			L.isInfo and L.log('MEC Client disabled')
			return
		self.stopMECSupport()
		self.startMECSupport()
		return

	@stop
	def stopMECSupport(self) -> None:
		L.isDebug and L.logDebug('Stopping MN-CSE Registration to MEC plugin')
		if not Configuration.mec_enable:
			L.isInfo and L.log('MEC Client disabled')
			return
		self.finishMECSupport()
		return

	@configure
	def configureMECSupport(self, config: Configuration) -> None:
		"""	Initialize the MEC client.
		"""
		L.isDebug and L.logDebug('configureMECSupport: Starting MN-CSE Registration to MEC plugin')
		parser = config.configParser
		config.mec_enable = parser.getboolean('etsi_mec', 'mec_enable', fallback = False)
		if not config.mec_enable:
			L.isInfo and L.log('configureMECSupport: MEC Client disabled')
			return
		config.mec_enable = parser.getboolean('etsi_mec', 'mec_enable', fallback = False)
		config.mec_host = parser.get('etsi_mec', 'mec_host', fallback = 'www.try-mec.etsi.org')
		config.mec_port = parser.getint('etsi_mec', 'mec_port', fallback = 443)
		config.cse_external_ip = parser.get('etsi_mec', 'cse_external_ip', fallback = 'www.try-mec.etsi.org')
		config.cse_external_port = parser.get('etsi_mec', 'cse_external_port', fallback = '80')
		config.mec_protocol = parser.get('etsi_mec', 'mec_protocol', fallback = 'https')
		config.mec_platform = parser.get('etsi_mec', 'mec_platform', fallback = 'mep1')
		config.mec_sandbox_id = parser.get('etsi_mec', 'mec_sandbox_id', fallback = 'mec_sandbox_id')
		config.fullAddress = parser.get('etsi_mec', 'fullAddress', fallback = None)
		config.mec_app_instance_id = parser.get('etsi_mec', 'mec_app_instance_id', fallback = None)

		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {config.mec_host}')
		self.http_address = normalizeURL(config.mec_host)
		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {self.http_address}')
		u = urlparse(self.http_address)
		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {u}')
		self.http_port = config.mec_port
		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {self.http_port}')
		self.mqtt_address = config.mqtt_address
		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {self.mqtt_address}')
		self.mqtt_port = config.mqtt_port
		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {self.mqtt_port}')
		self.use_wss = config.mqtt_websocket_enable
		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {self.use_wss}')
		self.wss_path = config.mqtt_websocket_path
		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {self.wss_path}')
		self.cse_resourceID = config.cse_resourceID
		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {self.cse_resourceID}')
		self.fullAddress = config.fullAddress
		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {self.fullAddress}')
		self.mec_app_instance_id = config.mec_app_instance_id
		L.isInfo and L.log(f'configureMECSupport: Call connectToMec with {self.mec_app_instance_id}')

		self.mecConnections:Dict[Tuple[str, int], Flask]	= {}
		""" Dictionary of MEC connections. """

		self.mecConnection = None
		""" The MEC connection. """

		self.thread = None
		""" The thread for the MEC connection. """

		L.isInfo and L.log('configureMECSupport: MEC Client: self.http_address=' + self.http_address)
		L.isInfo and L.log('configureMECSupport: MEC Client initialized')
		return

	@validate
	def validateMECSupport(self, config: Configuration) -> None:
		"""	Validate the configuration.

			Args:
				config: The configuration object.
				initial: If True, the configuration is validated for the first time.
		"""
		L.isDebug and L.logDebug('validateMECSupport: Starting MN-CSE Registration to MEC plugin')

		# override configuration with command line arguments
		if config._args_mec_enable is not None:
			config.mec_enable = config._args_mec_enable
		if config._args_mec_host is not None:
			config.mec_host = config._args_mec_host
		if config._args_mec_port is not None:
			config.mec_port = config._args_mec_port
		if config._args_mec_protocol is not None:
			config.mec_protocol = config._args_mec_protocol
		if config._args_mec_platform is not None:
			config.mec_platform = config._args_mec_platform
		if config._args_mec_sandbox_id is not None:
			config.mec_sandbox_id = config._args_mec_sandbox_id
		if config._args_cse_external_ip is not None:
			config.cse_external_ip = config._args_cse_external_ip
		if config._args_cse_external_port is not None:
			config.cse_external_port = config._args_cse_external_port
		if config._args_fullAddress is not None:
			config.fullAddress = config._args_fullAddress
		if config._args_mec_app_instance_id is not None:
			config.mec_app_instance_id = config._args_mec_app_instance_id
		#config.mec_host = normalizeURL(config.mec_host)

		return

	#
	#	Additional methods
	#

	def registerToMECPlatform(self):
		L.isDebug and L.logDebug(f'registerToMECPlatform: MEC enable: {Configuration.mec_enable}')
		if Configuration.mec_enable:
			self.confirm_readyToMec()
			self.post_mec_service()
		return

	def registerAsIoTPlatform(self):
		L.isDebug and L.logDebug(f'registerAsIoTPlatform: MEC enable: {Configuration.mec_enable}')
		if Configuration.mec_enable:
			self.registerToMec()
		return

	def connectToMec(self, host:str, port:int) -> Optional[Flask]:
		L.isDebug and L.logDebug(f'connectToMec: host: {host} - port: {port}')
		if Configuration.mec_enable:
			if not (mecConnect := self.mecConnections.get( (host, port) )):
				mecConnection = Flask(host)
				if mecConnection:
					self.mecConnections[(host, port)] = mecConnection
			L.isDebug and L.logDebug('<<< connectToMec')
			return mecConnection
		L.isDebug and L.logDebug('<<< connectToMec: None')
		return None

	
	def disconnectFromMec(self, host:str, port:int) -> None:
		"""	Remove the appropriate MECConnection for *host* and *port* from the 
			connection cache and also shut-down the connection.
		"""
		if (mecConnection := self.getMec(host, port)):
			if self.register is not None:
				self.deregisterFromMec()
			del self.mecConnections[ (host, port) ]
			# FIXME FSCOM Terminate Flask listener thread
	

	def getMec(self, host:str, port:int) -> Flask:
		"""	Return the MECConnection for the *host* and *port* from the internal
			connection cache.
		"""
		return self.mecConnections.get( (host, port) )


	#########################################################################
	#
	#	Send MEC requests
	#

	def registerToMec(self) -> Result:
		"""	Register to MEC platform.
		"""

		L.logDebug(f'>>> registerToMec')

		if self.isStopped:
			return Result(rsc = ResponseStatusCode.INTERNAL_SERVER_ERROR, 
						  dbg = 'MEC client is not running')

		try:
			url = \
				Configuration.mec_protocol + '://' + Configuration.mec_host + ':' + str(Configuration.mec_port) + \
				'/' + Configuration.mec_sandbox_id + \
				'/' + Configuration.mec_platform + \
				'/iots/v1/registered_iot_platforms'
			L.isInfo and L.log('MEC Client: registerToMec=' + url)
			body_json = {}
			body_json['iotPlatformId'] = str(uuid.uuid4())
			userTransportInfoList = []
			userTransportInfo = {}
			userTransportInfo['id'] = str(uuid.uuid4())
			userTransportInfo['name'] = Configuration.cse_cseID
			userTransportInfo['type'] = 'MB_TOPIC_BASED'
			if self.use_wss:
				userTransportInfo['description'] = 'MQTT over WS-Secured'
				userTransportInfo['protocol'] = 'MQTT+WSS'
			else:
				userTransportInfo['description'] = 'MQTT'
				userTransportInfo['protocol'] = 'MQTT'
			userTransportInfo['version'] = '2'
			userTransportInfo['endpoint'] = {}
			# userTransportInfo['endpoint']['addresses'] = []
			# The MEC API expects an array of URIs for the endpoint 'uris' field.
			# Previously this was set to a single string which caused the server
			# to return: "cannot unmarshal string into Go struct field ... uris of type []string".
			if self.use_wss:
				userTransportInfo['endpoint']['uris'] = []
				userTransportInfo['endpoint']['uris'].append('wss://' + self.mqtt_address + ':' + str(self.mqtt_port) + self.wss_path)
			else:
				userTransportInfo['endpoint']['addresses'] = []
				address = {}
				address['host'] = self.mqtt_address
				address['port'] = self.mqtt_port
				userTransportInfo['endpoint']['addresses'].append(address)
			userTransportInfo['security'] = {}
			userTransportInfo['implSpecificInfo'] = {}
			userTransportInfoList.append(userTransportInfo)
			body_json['userTransportInfo'] = userTransportInfoList
			customServicesTransportInfoList = []
			customServicesTransportInfo = {}
			# customServicesTransportInfo['id'] = str(uuid.uuid4())
			customServicesTransportInfo['id'] = Configuration.cse_cseID
			customServicesTransportInfo['name'] = Configuration.cse_resourceName
			customServicesTransportInfo['description'] = 'ACME oneM2M CSE ID'
			customServicesTransportInfo['type'] = 'REST_HTTP'
			customServicesTransportInfo['protocol'] = 'REST_HTTP'
			customServicesTransportInfo['version'] = '5'
			customServicesTransportInfo['endpoint'] = {}
			if self.fullAddress == '':
				customServicesTransportInfo['endpoint']['uris'] = [ 'https://' + self.cse_external_ip + ':' + str(self.http_port) ]
			else:
				customServicesTransportInfo['endpoint']['uris'] = [ self.fullAddress ]
			customServicesTransportInfo['security'] = {}
			customServicesTransportInfoList.append(customServicesTransportInfo)
			body_json['customServicesTransportInfo'] = customServicesTransportInfoList
			body_json['enabled'] = True
			L.isInfo and L.log('MEC Client: body_json=' + str(body_json))

			tries = 0
			while tries < 6:
				response = requests.post(url, headers=self.headers, json=body_json, verify=False)
				L.isInfo and L.log(f'MECClient.registerToMec: response.content: ' + str(response.content))
				if response.status_code == 201:
					self.register = Result()
					self.register.rsc = response.status_code
					self.register.data = response.content
					return self.register
				else:
					L.logErr(f'MECClient.registerToMec: ' + str(response.status_code))
					tries += 1 # Continue the loop
					time.sleep(10)
					L.isInfo and L.log('MEC Client: Status not 201, trying again...')
		except Exception as e:
			L.logErr(f'MECClient.registerToMec: ' + str(e))
		L.isInfo and L.log('MEC Client: Stopping MEC registration worker')
		self.mecRegistrationWorker.stop()
		self.mecRegistrationWorker = None
		return None


	def deregisterFromMec(self) -> Result:
		"""	Deregister to MEC platform.
		"""
		if self.isStopped:
			return Result(rsc = ResponseStatusCode.INTERNAL_SERVER_ERROR, dbg = 'MEC client is not running')

		if self.register is not None:
			try:
				json_dict = json.loads(self.register.data.decode('utf-8'))
				url = \
					Configuration.mec_protocol + '://' + Configuration.mec_host + ':' + str(Configuration.mec_port) + \
					'/' + Configuration.mec_sandbox_id + \
					'/' + Configuration.mec_platform + \
					'/iots/v1/registered_iot_platforms/' + json_dict['iotPlatformId']
				response = requests.delete(url, headers=self.headers, verify=False)
				self.register = None
				return Result(rsc = response.status_code)
			except Exception as e:
				L.logErr(f'MECClient.deregisterToMec: ' + str(e))
			
		self.register = None
		return Result(rsc = ResponseStatusCode.INTERNAL_SERVER_ERROR, dbg = 'MEC Deregistration failure')


	def confirm_readyToMec(self) -> Result:
		"""	Confirm readiness to MEC platform.
		"""
		if self.isStopped:
			return Result(rsc = ResponseStatusCode.INTERNAL_SERVER_ERROR, dbg = 'MEC client is not running')

		try:
			req_body = {
				"indication": "READY"
			}
			url = \
				Configuration.mec_protocol + '://' +Configuration.mec_host + ':' + str(Configuration.mec_port) + \
				'/' + Configuration.mec_sandbox_id + \
				'/' + Configuration.mec_platform + \
				'/mec_app_support/v2/applications/' + self.mec_app_instance_id + '/confirm_ready'
			L.log('MECClient.confirm_readyToMec: ' + str(req_body))
			L.log('MECClient.confirm_readyToMec: ' + str(url))
			response = requests.post(url, headers=self.headers, json=req_body, verify=False)
			return Result(rsc = response.status_code)
		except Exception as e:
			L.logErr(f'MECClient.confirm_readyToMec: ' + str(e))
			return Result(rsc = ResponseStatusCode.INTERNAL_SERVER_ERROR, dbg = 'MEC Confirm readiness failure')

	def post_mec_service(self) -> Result:
		"""
			Declare MEC service to MEC platform.
			This function declares MN-CSE as MEC service to the MEC platform.
		"""
		if self.isStopped:
			return Result(rsc = ResponseStatusCode.INTERNAL_SERVER_ERROR, dbg = 'MEC client is not running')

		try:
			req_body = {
				"serName": Configuration.cse_resourceName,
				"serCategory": {
					"href": "catalogueHref",
					"id": Configuration.cse_cseID,
					"name": Configuration.cse_resourceName,
					"version": "v1"
				},
				"version": "v1.0.0",
				"state": "ACTIVE",
				"transportInfo": {
					"id": "acme-mn-cse-transport",
					"name": "REST",
					"type": "REST_HTTP",
					"protocol": "HTTPS",
					"version": "5.0",
					"endpoint": {
						"uris": [
							self.fullAddress
						],
						"fqdn": None,
						"addresses": None,
						"alternative": None
					},
					"security": None
				},
				"serializer": "JSON",
				"scopeOfLocality": "MEC_HOST",
				"consumedLocalOnly": True
			}
			url = \
				Configuration.mec_protocol + '://' + Configuration.mec_host + ':' + str(Configuration.mec_port) + \
				'/' + Configuration.mec_sandbox_id + \
				'/' + Configuration.mec_platform + \
				'/mec_service_mgmt/v1/applications/' + self.mec_app_instance_id + '/services'
			L.log('MECClient.POST_mec_service: ' + str(req_body))
			L.log('MECClient.POST_mec_service: ' + str(url))
			response = requests.post(url, headers=self.headers, json=req_body, verify=False)
			return Result(rsc = response.status_code)
		except Exception as e:
			L.logErr(f'MECClient.POST_mec_service: ' + str(e))
			return Result(rsc = ResponseStatusCode.INTERNAL_SERVER_ERROR, dbg = 'MEC Declare MEC service failure')

