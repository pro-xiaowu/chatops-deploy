import {expect,test} from '@playwright/test'

test('local operator can log in and see provider choices',async({page},testInfo)=>{
  await page.goto('/')
  await page.getByRole('button',{name:'本地开发登录'}).click()
  await expect(page).toHaveURL(/dashboard/)
  await page.goto('/settings')
  await expect(page.getByText('飞书',{exact:true})).toBeVisible()
  await expect(page.getByText('企业微信',{exact:true})).toBeVisible()
  await expect(page.getByText('钉钉',{exact:true})).toBeVisible()
  await expect(page.getByText('仅 Web 控制台',{exact:true})).toBeVisible()
  await expect(page.locator('body')).not.toHaveCSS('overflow-x','scroll')
  await page.screenshot({path:testInfo.outputPath('settings.png'),fullPage:true})
})
