// Everything the admin area needs, in one module. The router imports it on
// demand (see router.tsx), so it becomes one separate download that
// customers browsing the shop never fetch.
export { AccountPage } from './AccountPage'
export { AdminLayout } from './AdminLayout'
export { AttributeEditPage } from './AttributeEditPage'
export { AttributesPage } from './AttributesPage'
export { AuditPage } from './AuditPage'
export { BrandsSuppliersPage } from './BrandsSuppliersPage'
export { CategoriesPage } from './CategoriesPage'
export { CategoryEditPage } from './CategoryEditPage'
export { DashboardPage } from './DashboardPage'
export { DevicesPage } from './DevicesPage'
export { ImportBatchPage } from './ImportBatchPage'
export { ImportsPage } from './ImportsPage'
export { LoginPage } from './LoginPage'
export { ProductEditorPage } from './ProductEditorPage'
export { ProductsPage } from './ProductsPage'
