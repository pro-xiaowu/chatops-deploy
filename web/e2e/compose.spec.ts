import {expect,test} from '@playwright/test'

test('local operator can log in and see provider choices',async({page},testInfo)=>{
  await page.goto('/')
  await page.getByRole('button',{name:'本地开发登录'}).click()
  await expect(page).toHaveURL(/dashboard/)
  await page.goto('/settings')
  const providerOptions=page.locator('.provider-options')
  await expect(providerOptions.getByText('飞书',{exact:true})).toBeVisible()
  await expect(providerOptions.getByText('企业微信',{exact:true})).toBeVisible()
  await expect(providerOptions.getByText('钉钉',{exact:true})).toBeVisible()
  await expect(providerOptions.getByText('仅 Web 控制台',{exact:true})).toBeVisible()
  await expect(page.locator('body')).not.toHaveCSS('overflow-x','scroll')
  await page.screenshot({path:testInfo.outputPath('settings.png'),fullPage:true})
})

test('environment management is reachable from an application row',async({page},testInfo)=>{
  await page.goto('/')
  await page.getByRole('button',{name:'本地开发登录'}).click()
  await expect(page).toHaveURL(/dashboard/)

  const csrf=(await page.context().cookies()).find(cookie=>cookie.name==='chatops_csrf')
  expect(csrf).toBeTruthy()
  const applicationName=`e2e-${testInfo.project.name}-${Date.now()}`
  const create=await page.request.post('/api/v1/applications',{
    headers:{'X-CSRF-Token':decodeURIComponent(csrf!.value)},
    data:{name:applicationName,description:'Playwright environment management'},
  })
  expect(create.ok(),await create.text()).toBeTruthy()

  await page.goto('/applications')
  const row=page.getByRole('row').filter({hasText:applicationName})
  await expect(row).toBeVisible()
  await row.getByRole('button',{name:'管理环境'}).click()

  const drawer=page.getByRole('dialog',{name:`${applicationName} 环境`})
  await expect(drawer).toBeVisible()
  await expect(drawer.getByText('请先在基础设施中注册 Kubernetes 集群')).toBeVisible()
  await expect(drawer.getByRole('button',{name:/添加环境/})).toBeDisabled()
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBeTruthy()
})
