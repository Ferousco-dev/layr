import Skeleton from '../../../components/Skeleton.jsx'

// ProfileSkeleton holds the page's shape while the profile loads.
export default function ProfileSkeleton() {
  const bar = (w, h = 14) => <Skeleton className="pill" style={{ width: w, height: h }} />
  return (
    <div aria-busy="true" aria-label="Loading your profile">
      <section className="panel identity">
        <div className="section-heading">
          {bar(120, 22)}
          {bar('80%')}
        </div>
        <div className="identity-details">
          <Skeleton className="round" style={{ width: 120, height: 120 }} />
          <div className="skeleton-stack">
            {bar(180, 22)}
            {bar(220)}
            {bar(160, 30)}
          </div>
        </div>
      </section>
      {[0, 1].map((i) => (
        <section className="panel" key={i}>
          <div className="section-heading">
            {bar(140, 22)}
            {bar('90%')}
          </div>
          <div className="skeleton-stack">
            {bar('100%', 44)}
            {bar('100%', 44)}
            {bar('100%', 44)}
          </div>
        </section>
      ))}
    </div>
  )
}
