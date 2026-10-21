import { Route, Routes } from 'react-router'
import { Layout } from './components/Layout.tsx'
import { Configurar } from './pages/Configurar.tsx'
import { NoEncontrada } from './pages/NoEncontrada.tsx'
import { Simulacion } from './pages/Simulacion.tsx'

export function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Configurar />} />
        <Route path="/simulacion/:id" element={<Simulacion />} />
        <Route path="*" element={<NoEncontrada />} />
      </Routes>
    </Layout>
  )
}
