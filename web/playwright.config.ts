import {defineConfig,devices} from '@playwright/test'

export default defineConfig({
  testDir:'./e2e',
  timeout:20_000,
  expect:{timeout:5_000},
  use:{baseURL:'http://localhost:8080',trace:'retain-on-failure',screenshot:'on'},
  projects:[
    {name:'desktop',use:{...devices['Desktop Chrome'],viewport:{width:1280,height:800}}},
    {name:'mobile',use:{...devices['Pixel 5'],viewport:{width:390,height:844}}},
  ],
})
