// Everything the admin area needs, in one module. The router imports it on
// demand (see router.tsx), so it becomes one separate download that
// customers browsing the shop never fetch.
export { AdminLayout } from './AdminLayout'
export { AttributeEditPage } from './AttributeEditPage'
export { AttributesPage } from './AttributesPage'
export { BrandsSuppliersPage } from './BrandsSuppliersPage'
export { CategoriesPage } from './CategoriesPage'
export { CategoryEditPage } from './CategoryEditPage'
export { DashboardPage } from './DashboardPage'
export { LoginPage } from './LoginPage'
export { ProductEditorPage } from './ProductEditorPage'
export { ProductsPage } from './ProductsPage'
