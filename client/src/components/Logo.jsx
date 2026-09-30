import logo from '../assets/brand/logo.webp'

// Logo is the Layr mark; its box keeps the real proportions (about 0.775 wide per unit of height).
export default function Logo({ height, ...rest }) {
  return <img src={logo} alt="" height={height} width={Math.round(height * 0.775)} style={{ height, width: 'auto' }} {...rest} />
}
