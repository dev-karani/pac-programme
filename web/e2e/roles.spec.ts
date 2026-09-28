import {test,expect} from '@playwright/test'

for(const role of ['student','supervisor','coordinator','hod','dean','leadership','support','examiner','admin'])test(`${role} workspace navigation loads without broken screens`,async({page})=>{
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));await page.goto('/');await page.getByLabel('Email address').fill(role+'@demo.pac.test');await page.getByLabel('Password',{exact:true}).fill('Demo123!Change');await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out'})).toBeVisible()
 const names=await page.getByRole('navigation',{name:'Main navigation'}).getByRole('button').allTextContents()
 for(const name of names){await page.getByRole('navigation',{name:'Main navigation'}).getByRole('button',{name,exact:true}).click();await expect(page.locator('.page h1').first()).toBeVisible();await expect(page.locator('.page .form-error')).toHaveCount(0)}
 expect(errors).toEqual([])
})
