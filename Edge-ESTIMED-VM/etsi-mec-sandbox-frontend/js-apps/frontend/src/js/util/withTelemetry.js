import React, { Component } from 'react';
import { faro } from '@grafana/faro-web-sdk';

export function withTelemetry(WrappedComponent, componentName) {
  return class extends Component {
    componentDidMount() {
      if (faro && faro.api) {
        faro.api.pushEvent('react_lifecycle', {
          component: componentName || WrappedComponent.name || 'UnknownComponent',
          lifecycle: 'MOUNTED'
        });
      }
    }

    componentWillUnmount() {
      if (faro && faro.api) {
        faro.api.pushEvent('react_lifecycle', {
          component: componentName || WrappedComponent.name || 'UnknownComponent',
          lifecycle: 'UNMOUNTED'
        });
      }
    }

    render() {
      return <WrappedComponent {...this.props} />;
    }
  };
}
