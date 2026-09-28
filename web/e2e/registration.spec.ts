import {test,expect} from '@playwright/test'
test('university-issued login replaces public registration',async({page,request})=>{
 await page.goto('/');await expect(page.getByRole('button',{name:'Create student account'})).toHaveCount(0)
 await expect(page.getByText('Accounts are issued by university IT.',{exact:false})).toBeVisible()
 const response=await request.post('/api/v1/auth/signup',{data:{Name:'Unauthorized account'}});expect(response.status()).toBe(403)
 await page.getByLabel('Email address').fill('newstudent@demo.pac.test');await page.getByLabel('Password',{exact:true}).fill('Demo123!Change');await page.getByRole('button',{name:'Sign in',exact:true}).click()
 await expect(page.getByRole('button',{name:'Choose supervisor'})).toBeVisible()
 await page.getByRole('button',{name:'My journey',exact:true}).click();await expect(page.locator('.journey-stage')).toHaveCount(4)
})
