/*
 * Copyright (c) 2022 ETSI.  All rights reserved.
 */

import React, { Component } from 'react';
import BasicDialog from './basic-dialog';

import CircularProgress from '@material-ui/core/CircularProgress';

import { withTelemetry } from '../../util/withTelemetry';

class SignInWaitDialog extends Component {
  constructor(props) {
    super(props);
    this.state = {
    };
  }

  render() {
    const isIpAddress = /^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$/.test(window.location.hostname);
    const isHttps = window.location.protocol === 'https:';
    const showSslWarning = isIpAddress && isHttps;

    return (
      <BasicDialog
        title={this.props.title}
        open={this.props.open}
        onClose={this.props.onClose}
        onSubmit={this.props.onClose}
        closeLabel={'Cancel'}
      >
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', margin: '20px 0' }}>
          <CircularProgress style={{ marginBottom: 20 }} />
          <p style={{ margin: '5px 0' }}>This may take a few seconds...</p>
          <p style={{ margin: '5px 0', color: 'gray' }}>You will automatically be redirected to the sandbox page when ready.</p>
        </div>

        {showSslWarning && (
          <div style={{ marginTop: '20px', padding: '15px', backgroundColor: '#fff8e1', border: '1px solid #ffb300', borderRadius: '4px' }}>
            <strong style={{ display: 'block', marginBottom: '10px', color: '#e65100' }}>Is this dialog stuck?</strong>
            <p style={{ marginBottom: '10px', color: '#555' }}>Your browser might be blocking the connection because of an untrusted SSL certificate. This stops sandbox from correctly working.</p>
            <strong style={{ display: 'block', marginBottom: '8px', color: '#555' }}>How to fix this in your browser:</strong>
            <ol style={{ paddingLeft: '20px', margin: 0, lineHeight: '1.5', color: '#555' }}>
              <li style={{ marginBottom: '8px' }}>Click the <strong>"Not secure"</strong> lock icon in your browser's address bar.</li>
              <li style={{ marginBottom: '8px' }}>Go to certificate details and export/save the certificate to a local file.</li>
              <li style={{ marginBottom: '8px' }}>Import it into your OS's <strong>"Trusted Root Certification Authorities"</strong> store.</li>
              <li>Restart your browser completely and sign in again.</li>
            </ol>
          </div>
        )}
      </BasicDialog>
    );
  }
}

export default withTelemetry(SignInWaitDialog, 'SignInWaitDialog');
