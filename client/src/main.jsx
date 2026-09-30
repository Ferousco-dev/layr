import { createRoot } from 'react-dom/client'
import App from './app/App.jsx'
import TopProgress from './components/TopProgress.jsx'
import './styles/base.css'

createRoot(document.getElementById('root')).render(
  <>
    <TopProgress />
    <App />
  </>,
)
