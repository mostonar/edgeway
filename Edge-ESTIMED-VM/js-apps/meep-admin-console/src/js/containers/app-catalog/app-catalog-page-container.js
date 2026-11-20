import React, { Component } from 'react';
import { connect } from 'react-redux';
import { Grid, GridCell } from '@rmwc/grid';
import { Typography } from '@rmwc/typography';
import { Button } from '@rmwc/button';
import { List, SimpleListItem } from '@rmwc/list';

class AppCatalogPageContainer extends Component {
  constructor(props) {
    super(props);
    this.state = {
      apps: []
    };
    this.fetchApps = this.fetchApps.bind(this);
    this.handleFileUpload = this.handleFileUpload.bind(this);
  }

  componentDidMount() {
    this.fetchApps();
  }

  fetchApps() {
    fetch('/virt-engine/v1/onboard_app')
      .then(res => res.json())
      .then(data => {
        this.setState({ apps: data || [] });
      })
      .catch(err => console.error("Failed to fetch app catalog:", err));
  };

  handleFileUpload(e) {
    const file = e.target.files[0];
    if (!file) return;

    // Simulate reading AppD
    const appDId = file.name.split('.')[0] + "-" + Math.floor(Math.random() * 1000);
    const mockAppD = {
      appDId: appDId,
      appName: "Uploaded App " + appDId,
      appDVersion: "1.0",
      appDescription: "Dynamically uploaded application"
    };

    const formData = new FormData();
    formData.append('helmChart', file);
    formData.append('appD', JSON.stringify(mockAppD));

    fetch('/virt-engine/v1/onboard_app', {
      method: 'POST',
      body: formData
    })
      .then(res => {
        if (res.ok) {
          alert('App uploaded successfully');
          this.fetchApps();
        } else {
          alert('Upload failed');
        }
      })
      .catch(err => {
        console.error(err);
        alert('Upload failed: ' + err.message);
      });
  };

  render() {
    return (
      <div style={{ padding: 20, width: '100%' }}>
        <Grid>
          <GridCell span={12}>
            <Typography use="headline4">Dynamic Application Catalog - Experimental</Typography>
          </GridCell>

          <GridCell span={12} style={{ marginTop: 20 }}>
            <Typography use="headline6">Onboard New Application</Typography>
            <div style={{ marginTop: 10 }}>
              <input
                type="file"
                accept=".tgz"
                onChange={this.handleFileUpload}
                style={{ display: 'none' }}
                id="helm-chart-upload"
              />
              <Button raised onClick={() => document.getElementById('helm-chart-upload').click()}>
                Upload Helm Chart (.tgz)
              </Button>
            </div>
          </GridCell>

          <GridCell span={12} style={{ marginTop: 40 }}>
            <Typography use="headline6">Onboarded Applications</Typography>
            <List twoLine style={{ marginTop: 10, background: '#fff', border: '1px solid #ccc' }}>
              {this.state.apps.map((app, i) => (
                <SimpleListItem
                  key={i}
                  text={app.appName}
                  secondaryText={`ID: ${app.appDId} | Version: ${app.appDVersion}`}
                />
              ))}
              {this.state.apps.length === 0 && (
                <div style={{ padding: 20 }}>No applications onboarded yet.</div>
              )}
            </List>
          </GridCell>
        </Grid>
      </div>
    );
  }
}

const mapStateToProps = state => {
  return {};
};

const mapDispatchToProps = dispatch => {
  return {};
};

export default connect(
  mapStateToProps,
  mapDispatchToProps
)(AppCatalogPageContainer);
