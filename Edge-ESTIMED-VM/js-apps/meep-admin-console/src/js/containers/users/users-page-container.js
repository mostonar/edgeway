import React, { Component } from 'react';
import { connect } from 'react-redux';
import { Grid, GridCell, GridInner } from '@rmwc/grid';
import { Typography } from '@rmwc/typography';
import { Button } from '@rmwc/button';
import { List, SimpleListItem } from '@rmwc/list';
import { TextField } from '@rmwc/textfield';
import { Select } from '@rmwc/select';
import { Card } from '@rmwc/card';
import * as meepAuthSvcRestApiClient from '../../../../../../js-packages/meep-auth-svc-client/src/index.js';

class UsersPageContainer extends Component {
  constructor(props) {
    super(props);
    this.state = {
      users: [],
      newProvider: 'local',
      newUsername: '',
      newPassword: '',
      newRole: 'admin',
      newSboxname: this.generateSandboxName(),
      isValidating: false
    };

    // Initialize Auth API client
    this.authApi = new meepAuthSvcRestApiClient.AuthApi();

    this.fetchUsers = this.fetchUsers.bind(this);
    this.handleDelete = this.handleDelete.bind(this);
    this.handleRoleToggle = this.handleRoleToggle.bind(this);
    this.handleAddUser = this.handleAddUser.bind(this);
  }

  generateSandboxName() {
    const charset = 'abcdefghijklmnopqrstuvwxyz0123456789';
    let randSeq = '';
    for (let i = 0; i < 7; i++) {
      randSeq += charset.charAt(Math.floor(Math.random() * charset.length));
    }
    return 'sbx' + randSeq;
  }

  componentDidMount() {
    this.fetchUsers();
  }

  fetchUsers() {
    this.authApi.getUsers((error, data, response) => {
      if (error) {
        console.error("Failed to fetch users:", error);
      } else {
        this.setState({ users: data || [] });
      }
    });
  }

  handleDelete(provider, username) {
    if (window.confirm(`Are you sure you want to delete user ${username} (${provider})?`)) {
      this.authApi.deleteUser(provider, username, (error, data, response) => {
        if (error) {
          alert('Failed to delete user: ' + error.message);
        } else {
          this.fetchUsers();
        }
      });
    }
  }

  handleRoleToggle(provider, username, currentRole) {
    const newRole = currentRole === 'admin' ? 'user' : 'admin';
    if (window.confirm(`Are you sure you want to change role of ${username} to ${newRole}?`)) {
      this.authApi.updateUserRole(provider, username, { role: newRole }, (error, data, response) => {
        if (error) {
          alert('Failed to update role: ' + error.message);
        } else {
          this.fetchUsers();
        }
      });
    }
  }

  async handleAddUser() {
    const { newProvider, newUsername, newPassword, newRole, newSboxname } = this.state;
    if (!newUsername) {
      alert("Username is required.");
      return;
    }
    if (newProvider === 'local' && !newPassword) {
      alert("Password is required for local users.");
      return;
    }

    this.setState({ isValidating: false });

    const user = {
      provider: 'local',
      username: newUsername,
      password: newPassword,
      role: 'admin',
      sboxname: newSboxname
    };

    this.authApi.createUser(user, (error, data, response) => {
      this.setState({ isValidating: false });
      if (error) {
        alert('Failed to create user: ' + error.message);
      } else {
        this.setState({ 
          newUsername: '', 
          newPassword: '', 
          newSboxname: this.generateSandboxName() 
        });
        this.fetchUsers();
      }
    });
  }

  render() {
    const isLocal = true;

    return (
      <div style={{ padding: 20, width: '100%', maxWidth: '1200px', margin: '0 auto' }}>
        <Grid>
          <GridCell span={12}>
            <Typography use="headline4">User Management</Typography>
          </GridCell>

          {/* Add User Card */}
          <GridCell span={12} style={{ marginTop: 20 }}>
            <Card style={{ padding: 20, backgroundColor: '#f5f5f5', border: '1px solid #ddd' }}>
              <Typography use="headline6" style={{ marginBottom: 15 }}>Add New User</Typography>
              <form onSubmit={(e) => { e.preventDefault(); this.handleAddUser(); }}>
                <GridInner>
                  <GridCell span={3}>
                    <TextField
                      label="Username"
                      autoComplete="username"
                      value={this.state.newUsername}
                      onChange={(e) => this.setState({ newUsername: e.target.value })}
                      style={{ width: '100%' }}
                    />
                  </GridCell>
                  <GridCell span={3}>
                    <TextField
                      label="Password"
                      type="password"
                      autoComplete="new-password"
                      value={this.state.newPassword}
                      onChange={(e) => this.setState({ newPassword: e.target.value })}
                      style={{ width: '100%' }}
                    />
                  </GridCell>
                  <GridCell span={2}>
                    <TextField
                      label="Sandbox Name"
                      value={this.state.newSboxname}
                      readOnly
                      disabled
                      style={{ width: '100%' }}
                    />
                  </GridCell>
                  <GridCell span={2}>
                    <Select
                      label="Role"
                      value="admin"
                      options={['admin']}
                      disabled
                      style={{ width: '100%' }}
                    />
                  </GridCell>
                  <GridCell span={2} style={{ display: 'flex', alignItems: 'center' }}>
                    <Button 
                      unelevated 
                      style={{ width: '100%', height: '40px' }} 
                      type="submit"
                      disabled={!this.state.newUsername || !this.state.newPassword || this.state.isValidating}
                    >
                      {this.state.isValidating ? "VERIFYING..." : "ADD USER"}
                    </Button>
                  </GridCell>
                </GridInner>
              </form>
            </Card>
          </GridCell>

          <GridCell span={12} style={{ marginTop: 20 }}>
            <Typography use="headline6" style={{ marginBottom: 15 }}>Existing Users</Typography>
            <List twoLine style={{ background: '#fff', border: '1px solid #ccc', borderRadius: '4px' }}>
              {this.state.users.map((u, i) => (
                <SimpleListItem
                  key={i}
                  text={u.username}
                  secondaryText={`Provider: ${u.provider} | Role: ${u.role} | Sandbox: ${u.sboxname || 'None'}`}
                  meta={
                    <div style={{ display: 'flex', gap: '10px' }}>
                      <Button outlined onClick={() => this.handleRoleToggle(u.provider, u.username, u.role)}>
                        {u.role === 'admin' ? 'Demote to User' : 'Promote to Admin'}
                      </Button>
                      <Button unelevated style={{ backgroundColor: '#b00020', color: 'white' }} onClick={() => this.handleDelete(u.provider, u.username)}>
                        Delete
                      </Button>
                    </div>
                  }
                />
              ))}
              {this.state.users.length === 0 && (
                <div style={{ padding: 20 }}>
                  <Typography use="body1">No users found.</Typography>
                </div>
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
)(UsersPageContainer);
