export interface SchoolBranding {
  name: string
  app_name: string
  logo_url: string
}
export interface SchoolSettings extends SchoolBranding {
  address: string
  phone: string
  email: string
}
