import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'com.cryon.app',
  appName: 'Cryon',
  webDir: 'dist',
  server: {
    androidScheme: 'https'
  }
};

export default config;
